package auth

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// TestPropertySearchFilterReturnsOnlyMatchingUsers verifies Property 1:
// Search filter returns only matching users.
//
// For any set of users and any non-empty search string, listing users with
// that search term should return only users whose email or name contains the
// search string (case-insensitive), and no matching user should be excluded
// from the results.
func TestPropertySearchFilterReturnsOnlyMatchingUsers(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		repo := newMockUserRepo()

		// Generate a random set of users (0 to 20 users).
		numUsers := rapid.IntRange(0, 20).Draw(t, "numUsers")
		var allUsers []User

		for i := 0; i < numUsers; i++ {
			user := User{
				ID:        uuid.New(),
				Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
				Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
				Role:      rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
				Disabled:  rapid.Bool().Draw(t, "disabled"),
				CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
				UpdatedAt: time.Now(),
			}
			allUsers = append(allUsers, user)
			_ = repo.Create(ctx, &user)
		}

		// Generate a search term. We'll use a substring from one of the users
		// or a random string to test both matching and non-matching cases.
		var searchTerm string
		if numUsers > 0 && rapid.Bool().Draw(t, "useExistingSubstring") {
			// Pick a random user and extract a substring from email or name.
			idx := rapid.IntRange(0, numUsers-1).Draw(t, "userIdx")
			if rapid.Bool().Draw(t, "fromEmail") {
				// Extract substring from email.
				email := allUsers[idx].Email
				if len(email) > 2 {
					start := rapid.IntRange(0, len(email)-2).Draw(t, "substringStart")
					end := rapid.IntRange(start+1, len(email)).Draw(t, "substringEnd")
					searchTerm = email[start:end]
				} else {
					searchTerm = email
				}
			} else {
				// Extract substring from name.
				name := allUsers[idx].Name
				if len(name) > 2 {
					start := rapid.IntRange(0, len(name)-2).Draw(t, "substringStart")
					end := rapid.IntRange(start+1, len(name)).Draw(t, "substringEnd")
					searchTerm = name[start:end]
				} else {
					searchTerm = name
				}
			}
		} else {
			// Generate a random search term.
			searchTerm = rapid.StringMatching(`[a-z0-9]{1,8}`).Draw(t, "randomSearch")
		}

		// Call ListWithFilters with the search term.
		results, totalCount, err := repo.ListWithFilters(ctx, searchTerm, "", "", 100, 0)
		if err != nil {
			t.Fatalf("ListWithFilters returned error: %v", err)
		}

		// Verify that all returned users match the search term.
		searchLower := strings.ToLower(searchTerm)
		for _, u := range results {
			emailLower := strings.ToLower(u.Email)
			nameLower := strings.ToLower(u.Name)
			if !strings.Contains(emailLower, searchLower) && !strings.Contains(nameLower, searchLower) {
				t.Fatalf("User %s (email=%s, name=%s) was returned but does not match search term %q",
					u.ID, u.Email, u.Name, searchTerm)
			}
		}

		// Verify that no matching user was excluded.
		expectedCount := 0
		for _, u := range allUsers {
			emailLower := strings.ToLower(u.Email)
			nameLower := strings.ToLower(u.Name)
			if strings.Contains(emailLower, searchLower) || strings.Contains(nameLower, searchLower) {
				expectedCount++
			}
		}

		if len(results) != expectedCount {
			t.Fatalf("Expected %d matching users, but got %d", expectedCount, len(results))
		}

		if totalCount != expectedCount {
			t.Fatalf("Expected total count %d, but got %d", expectedCount, totalCount)
		}
	})
}

// TestPropertyRoleFilterReturnsOnlyUsersWithSpecifiedRole verifies Property 2:
// Role filter returns only users with the specified role.
//
// For any set of users with mixed roles and any valid role value (user, premium,
// admin), listing users filtered by that role should return only users whose role
// matches exactly, and no matching user should be excluded.
func TestPropertyRoleFilterReturnsOnlyUsersWithSpecifiedRole(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		repo := newMockUserRepo()

		// Generate a random set of users (0 to 30 users) with mixed roles.
		numUsers := rapid.IntRange(0, 30).Draw(t, "numUsers")
		var allUsers []User

		for i := 0; i < numUsers; i++ {
			user := User{
				ID:        uuid.New(),
				Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
				Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
				Role:      rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
				Disabled:  rapid.Bool().Draw(t, "disabled"),
				CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
				UpdatedAt: time.Now(),
			}
			allUsers = append(allUsers, user)
			_ = repo.Create(ctx, &user)
		}

		// Pick a role to filter by.
		roleFilter := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "roleFilter")

		// Call ListWithFilters with the role filter.
		results, totalCount, err := repo.ListWithFilters(ctx, "", roleFilter, "", 100, 0)
		if err != nil {
			t.Fatalf("ListWithFilters returned error: %v", err)
		}

		// Verify that all returned users have the specified role.
		for _, u := range results {
			if u.Role != roleFilter {
				t.Fatalf("User %s has role %q but was returned when filtering for role %q",
					u.ID, u.Role, roleFilter)
			}
		}

		// Verify that no matching user was excluded.
		expectedCount := 0
		for _, u := range allUsers {
			if u.Role == roleFilter {
				expectedCount++
			}
		}

		if len(results) != expectedCount {
			t.Fatalf("Expected %d users with role %q, but got %d", expectedCount, roleFilter, len(results))
		}

		if totalCount != expectedCount {
			t.Fatalf("Expected total count %d for role %q, but got %d", expectedCount, roleFilter, totalCount)
		}
	})
}

// TestPropertyStatusFilterReturnsOnlyUsersMatchingDisabledState verifies Property 3:
// Status filter returns only users matching the requested disabled state.
//
// For any set of users with mixed disabled states, listing users with status=active
// should return only users where disabled=false, and listing with status=disabled
// should return only users where disabled=true. In both cases, no matching user
// should be excluded.
func TestPropertyStatusFilterReturnsOnlyUsersMatchingDisabledState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		repo := newMockUserRepo()

		// Generate a random set of users (0 to 30 users) with mixed disabled states.
		numUsers := rapid.IntRange(0, 30).Draw(t, "numUsers")
		var allUsers []User

		for i := 0; i < numUsers; i++ {
			user := User{
				ID:        uuid.New(),
				Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
				Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
				Role:      rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
				Disabled:  rapid.Bool().Draw(t, "disabled"),
				CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
				UpdatedAt: time.Now(),
			}
			allUsers = append(allUsers, user)
			_ = repo.Create(ctx, &user)
		}

		// Pick a status filter: "active" or "disabled".
		statusFilter := rapid.SampledFrom([]string{"active", "disabled"}).Draw(t, "statusFilter")

		// Call ListWithFilters with the status filter.
		results, totalCount, err := repo.ListWithFilters(ctx, "", "", statusFilter, 100, 0)
		if err != nil {
			t.Fatalf("ListWithFilters returned error: %v", err)
		}

		// Determine the expected disabled state based on the filter.
		expectedDisabled := (statusFilter == "disabled")

		// Verify that all returned users have the correct disabled state.
		for _, u := range results {
			if u.Disabled != expectedDisabled {
				t.Fatalf("User %s has disabled=%v but was returned when filtering for status=%q",
					u.ID, u.Disabled, statusFilter)
			}
		}

		// Verify that no matching user was excluded.
		expectedCount := 0
		for _, u := range allUsers {
			if u.Disabled == expectedDisabled {
				expectedCount++
			}
		}

		if len(results) != expectedCount {
			t.Fatalf("Expected %d users with status=%q (disabled=%v), but got %d",
				expectedCount, statusFilter, expectedDisabled, len(results))
		}

		if totalCount != expectedCount {
			t.Fatalf("Expected total count %d for status=%q, but got %d",
				expectedCount, statusFilter, totalCount)
		}
	})
}

// TestPropertyPaginationReturnsCorrectSliceAndTotalCount verifies Property 4:
// Pagination returns the correct slice and accurate total count.
//
// For any set of N users and any valid page/limit combination, the returned list
// should contain at most `limit` users starting at offset `(page-1)*limit`, the
// total count should equal the number of users matching the current filters, and
// every returned user should include all required fields (id, email, name,
// avatar_url, role, has_google, has_password, disabled, must_change_password,
// created_at, updated_at).
func TestPropertyPaginationReturnsCorrectSliceAndTotalCount(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		repo := newMockUserRepo()

		// Generate a random set of users (0 to 50 users).
		numUsers := rapid.IntRange(0, 50).Draw(t, "numUsers")
		var allUsers []User

		for i := 0; i < numUsers; i++ {
			user := User{
				ID:                 uuid.New(),
				Email:              rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
				Name:               rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
				AvatarURL:          rapid.StringMatching(`https?://[a-z0-9\-\.]+/[a-z0-9\-]+\.(jpg|png)`).Draw(t, "avatarURL"),
				Role:               rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
				Disabled:           rapid.Bool().Draw(t, "disabled"),
				MustChangePassword: rapid.Bool().Draw(t, "mustChangePassword"),
				CreatedAt:          time.Now().Add(-time.Duration(i) * time.Hour),
				UpdatedAt:          time.Now(),
			}

			// Randomly assign Google ID and password hash to test has_google and has_password.
			if rapid.Bool().Draw(t, "hasGoogle") {
				googleID := rapid.StringMatching(`[0-9]{15,25}`).Draw(t, "googleID")
				user.GoogleID = &googleID
			}
			if rapid.Bool().Draw(t, "hasPassword") {
				passwordHash := rapid.StringMatching(`\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}`).Draw(t, "passwordHash")
				user.PasswordHash = &passwordHash
			}

			allUsers = append(allUsers, user)
			_ = repo.Create(ctx, &user)
		}

		// Generate optional filters to test pagination with filtering.
		var searchTerm, roleFilter, statusFilter string

		// Optionally add a search filter.
		if numUsers > 0 && rapid.Bool().Draw(t, "useSearch") {
			idx := rapid.IntRange(0, numUsers-1).Draw(t, "searchUserIdx")
			if rapid.Bool().Draw(t, "searchEmail") {
				email := allUsers[idx].Email
				if len(email) > 2 {
					start := rapid.IntRange(0, len(email)-2).Draw(t, "searchStart")
					end := rapid.IntRange(start+1, len(email)).Draw(t, "searchEnd")
					searchTerm = email[start:end]
				} else {
					searchTerm = email
				}
			} else {
				name := allUsers[idx].Name
				if len(name) > 2 {
					start := rapid.IntRange(0, len(name)-2).Draw(t, "searchStart")
					end := rapid.IntRange(start+1, len(name)).Draw(t, "searchEnd")
					searchTerm = name[start:end]
				} else {
					searchTerm = name
				}
			}
		}

		// Optionally add a role filter.
		if rapid.Bool().Draw(t, "useRoleFilter") {
			roleFilter = rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "roleFilter")
		}

		// Optionally add a status filter.
		if rapid.Bool().Draw(t, "useStatusFilter") {
			statusFilter = rapid.SampledFrom([]string{"active", "disabled"}).Draw(t, "statusFilter")
		}

		// Generate valid page and limit values.
		// Page: 1 to 10, Limit: 1 to 20.
		page := rapid.IntRange(1, 10).Draw(t, "page")
		limit := rapid.IntRange(1, 20).Draw(t, "limit")

		// Calculate offset.
		offset := (page - 1) * limit

		// Call ListWithFilters with pagination parameters.
		results, totalCount, err := repo.ListWithFilters(ctx, searchTerm, roleFilter, statusFilter, limit, offset)
		if err != nil {
			t.Fatalf("ListWithFilters returned error: %v", err)
		}

		// Manually compute the expected filtered users to verify correctness.
		var expectedFiltered []User
		for _, u := range allUsers {
			// Apply search filter.
			if searchTerm != "" {
				searchLower := strings.ToLower(searchTerm)
				emailLower := strings.ToLower(u.Email)
				nameLower := strings.ToLower(u.Name)
				if !strings.Contains(emailLower, searchLower) && !strings.Contains(nameLower, searchLower) {
					continue
				}
			}

			// Apply role filter.
			if roleFilter != "" && u.Role != roleFilter {
				continue
			}

			// Apply status filter.
			if statusFilter == "active" && u.Disabled {
				continue
			}
			if statusFilter == "disabled" && !u.Disabled {
				continue
			}

			expectedFiltered = append(expectedFiltered, u)
		}

		// Sort expectedFiltered by created_at DESC to match repository behavior.
		// Note: The mock repo sorts by created_at DESC.
		// We need to replicate this sorting to compare slices.
		// Since we created users with decreasing created_at (i * hour ago),
		// the first user created has the oldest timestamp, and the last has the newest.
		// Sorting DESC means newest first.
		// In our allUsers slice, index 0 has the newest created_at (now - 0*hour),
		// and index numUsers-1 has the oldest (now - (numUsers-1)*hour).
		// So expectedFiltered should already be in the correct order if we iterate allUsers in order.
		// But to be safe, let's sort expectedFiltered explicitly.
		sort.Slice(expectedFiltered, func(i, j int) bool {
			return expectedFiltered[i].CreatedAt.After(expectedFiltered[j].CreatedAt)
		})

		expectedTotalCount := len(expectedFiltered)

		// Verify total count matches.
		if totalCount != expectedTotalCount {
			t.Fatalf("Expected total count %d, but got %d (search=%q, role=%q, status=%q)",
				expectedTotalCount, totalCount, searchTerm, roleFilter, statusFilter)
		}

		// Compute the expected page slice.
		var expectedPage []User
		if offset < len(expectedFiltered) {
			end := offset + limit
			if end > len(expectedFiltered) {
				end = len(expectedFiltered)
			}
			expectedPage = expectedFiltered[offset:end]
		}

		// Verify the returned results match the expected page.
		if len(results) != len(expectedPage) {
			t.Fatalf("Expected %d users in page %d (limit %d, offset %d), but got %d (search=%q, role=%q, status=%q)",
				len(expectedPage), page, limit, offset, len(results), searchTerm, roleFilter, statusFilter)
		}

		// Verify each returned user matches the expected user and has all required fields.
		for i, result := range results {
			expected := expectedPage[i]

			// Check that IDs match (this ensures we got the right user in the right order).
			if result.ID != expected.ID {
				t.Fatalf("User at index %d: expected ID %s, got %s", i, expected.ID, result.ID)
			}

			// Verify all required fields are present and match.
			if result.Email != expected.Email {
				t.Fatalf("User %s: expected email %q, got %q", result.ID, expected.Email, result.Email)
			}
			if result.Name != expected.Name {
				t.Fatalf("User %s: expected name %q, got %q", result.ID, expected.Name, result.Name)
			}
			if result.AvatarURL != expected.AvatarURL {
				t.Fatalf("User %s: expected avatar_url %q, got %q", result.ID, expected.AvatarURL, result.AvatarURL)
			}
			if result.Role != expected.Role {
				t.Fatalf("User %s: expected role %q, got %q", result.ID, expected.Role, result.Role)
			}
			if result.Disabled != expected.Disabled {
				t.Fatalf("User %s: expected disabled %v, got %v", result.ID, expected.Disabled, result.Disabled)
			}
			if result.MustChangePassword != expected.MustChangePassword {
				t.Fatalf("User %s: expected must_change_password %v, got %v", result.ID, expected.MustChangePassword, result.MustChangePassword)
			}

			// Verify has_google and has_password are correctly derived.
			expectedHasGoogle := (expected.GoogleID != nil)
			resultHasGoogle := (result.GoogleID != nil)
			if resultHasGoogle != expectedHasGoogle {
				t.Fatalf("User %s: expected has_google %v, got %v", result.ID, expectedHasGoogle, resultHasGoogle)
			}

			expectedHasPassword := (expected.PasswordHash != nil)
			resultHasPassword := (result.PasswordHash != nil)
			if resultHasPassword != expectedHasPassword {
				t.Fatalf("User %s: expected has_password %v, got %v", result.ID, expectedHasPassword, resultHasPassword)
			}

			// Verify created_at and updated_at are present (non-zero).
			if result.CreatedAt.IsZero() {
				t.Fatalf("User %s: created_at is zero", result.ID)
			}
			if result.UpdatedAt.IsZero() {
				t.Fatalf("User %s: updated_at is zero", result.ID)
			}
		}
	})
}

// TestPropertyPaginationReturnsCorrectSliceAndTotalCount verifies Property 4:
// Pagination returns the correct slice and accurate total count.
//
// For any set of N users and any valid page/limit combination, the returned list
// should contain at most `limit` users starting at offset `(page-1)*limit`, the

// TestPropertyValidRoleUpdatePersistsNewRole verifies Property 5:
// Valid role update persists the new role.
//
// For any user and any valid role value (user, premium, admin), updating the
// user's role should result in the returned user record having the new role,
// and a subsequent fetch of that user should confirm the role was persisted.
func TestPropertyValidRoleUpdatePersistsNewRole(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create a target user with a random initial role.
		targetUser := User{
			ID:        uuid.New(),
			Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
			Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
			Role:      rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "initialRole"),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &targetUser)

		// Create an acting admin (different from target user).
		actingAdmin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &actingAdmin)

		// Pick a new role (may be the same as initial role, which is valid).
		newRole := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "newRole")

		// Update the role.
		updatedUser, err := service.UpdateRole(ctx, actingAdmin.ID, targetUser.ID, newRole)
		if err != nil {
			t.Fatalf("UpdateRole returned error: %v", err)
		}

		// Verify the returned user has the new role.
		if updatedUser.Role != newRole {
			t.Fatalf("Expected role %q in returned user, got %q", newRole, updatedUser.Role)
		}

		// Fetch the user again to confirm persistence.
		fetchedUser, err := userRepo.FindByID(ctx, targetUser.ID)
		if err != nil {
			t.Fatalf("FindByID returned error: %v", err)
		}
		if fetchedUser == nil {
			t.Fatalf("User not found after role update")
			return
		}
		if fetchedUser.Role != newRole {
			t.Fatalf("Expected role %q in fetched user, got %q", newRole, fetchedUser.Role)
		}
	})
}

// TestPropertyInvalidRoleValuesAreRejected verifies Property 6:
// Invalid role values are rejected.
//
// For any string that is not one of user, premium, or admin, attempting to
// update a user's role to that value should return an error and leave the
// user's role unchanged.
func TestPropertyInvalidRoleValuesAreRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create a target user with a valid initial role.
		initialRole := rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "initialRole")
		targetUser := User{
			ID:        uuid.New(),
			Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
			Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
			Role:      initialRole,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &targetUser)

		// Create an acting admin.
		actingAdmin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &actingAdmin)

		// Generate an invalid role value (anything except user, premium, admin).
		// We'll use a random string that's not one of the valid roles.
		invalidRole := rapid.StringMatching(`[a-z]{3,15}`).
			Filter(func(s string) bool {
				return s != "user" && s != "premium" && s != "admin"
			}).
			Draw(t, "invalidRole")

		// Attempt to update the role with the invalid value.
		_, err := service.UpdateRole(ctx, actingAdmin.ID, targetUser.ID, invalidRole)

		// Verify that an error was returned.
		if err == nil {
			t.Fatalf("Expected error for invalid role %q, but got nil", invalidRole)
		}

		// Verify the error is ErrInvalidRole.
		if err != ErrInvalidRole {
			t.Fatalf("Expected ErrInvalidRole, got %v", err)
		}

		// Verify the user's role was not changed.
		fetchedUser, err := userRepo.FindByID(ctx, targetUser.ID)
		if err != nil {
			t.Fatalf("FindByID returned error: %v", err)
		}
		if fetchedUser == nil {
			t.Fatalf("User not found after invalid role update attempt")
			return
		}
		if fetchedUser.Role != initialRole {
			t.Fatalf("User role changed from %q to %q after invalid role update", initialRole, fetchedUser.Role)
		}
	})
}

// TestPropertyDisableThenReEnableRestoresOriginalState verifies Property 7:
// Disable then re-enable restores original state.
//
// For any user that is initially enabled, disabling the user should result in
// disabled=true, and subsequently re-enabling should result in disabled=false.
// The user's other fields (role, email, name, etc.) should remain unchanged
// through both operations.
func TestPropertyDisableThenReEnableRestoresOriginalState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create a target user that is initially enabled.
		targetUser := User{
			ID:        uuid.New(),
			Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
			Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
			Role:      rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
			Disabled:  false,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &targetUser)

		// Create an acting admin.
		actingAdmin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &actingAdmin)

		// Disable the user.
		disabledUser, err := service.UpdateDisabled(ctx, actingAdmin.ID, targetUser.ID, true)
		if err != nil {
			t.Fatalf("UpdateDisabled(true) returned error: %v", err)
		}

		// Verify the user is now disabled.
		if !disabledUser.Disabled {
			t.Fatalf("Expected disabled=true after disabling, got false")
		}

		// Verify other fields remain unchanged.
		if disabledUser.Email != targetUser.Email {
			t.Fatalf("Email changed from %q to %q after disable", targetUser.Email, disabledUser.Email)
		}
		if disabledUser.Name != targetUser.Name {
			t.Fatalf("Name changed from %q to %q after disable", targetUser.Name, disabledUser.Name)
		}
		if disabledUser.Role != targetUser.Role {
			t.Fatalf("Role changed from %q to %q after disable", targetUser.Role, disabledUser.Role)
		}

		// Re-enable the user.
		enabledUser, err := service.UpdateDisabled(ctx, actingAdmin.ID, targetUser.ID, false)
		if err != nil {
			t.Fatalf("UpdateDisabled(false) returned error: %v", err)
		}

		// Verify the user is now enabled.
		if enabledUser.Disabled {
			t.Fatalf("Expected disabled=false after re-enabling, got true")
		}

		// Verify other fields remain unchanged.
		if enabledUser.Email != targetUser.Email {
			t.Fatalf("Email changed from %q to %q after re-enable", targetUser.Email, enabledUser.Email)
		}
		if enabledUser.Name != targetUser.Name {
			t.Fatalf("Name changed from %q to %q after re-enable", targetUser.Name, enabledUser.Name)
		}
		if enabledUser.Role != targetUser.Role {
			t.Fatalf("Role changed from %q to %q after re-enable", targetUser.Role, enabledUser.Role)
		}
	})
}

// TestPropertyDisablingUserInvalidatesAllSessions verifies Property 8:
// Disabling a user invalidates all their sessions.
//
// For any user with one or more active sessions, disabling that user should
// result in all sessions for that user being deleted. A subsequent session
// lookup for that user should return zero results.
func TestPropertyDisablingUserInvalidatesAllSessions(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create a target user.
		targetUser := User{
			ID:        uuid.New(),
			Email:     rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
			Name:      rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
			Role:      rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
			Disabled:  false,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &targetUser)

		// Create an acting admin.
		actingAdmin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &actingAdmin)

		// Create 1 to 5 sessions for the target user.
		numSessions := rapid.IntRange(1, 5).Draw(t, "numSessions")
		for i := 0; i < numSessions; i++ {
			session := Session{
				ID:               uuid.New(),
				UserID:           targetUser.ID,
				RefreshTokenHash: rapid.StringMatching(`[a-f0-9]{64}`).Draw(t, "tokenHash"),
				ExpiresAt:        time.Now().Add(24 * time.Hour),
				CreatedAt:        time.Now(),
			}
			_ = sessionRepo.Create(ctx, &session)
		}

		// Verify sessions exist before disabling.
		sessionCountBefore := sessionRepo.sessionsForUser(targetUser.ID)
		if sessionCountBefore != numSessions {
			t.Fatalf("Expected %d sessions before disable, got %d", numSessions, sessionCountBefore)
		}

		// Disable the user.
		_, err := service.UpdateDisabled(ctx, actingAdmin.ID, targetUser.ID, true)
		if err != nil {
			t.Fatalf("UpdateDisabled returned error: %v", err)
		}

		// Verify all sessions were deleted.
		sessionCountAfter := sessionRepo.sessionsForUser(targetUser.ID)
		if sessionCountAfter != 0 {
			t.Fatalf("Expected 0 sessions after disable, got %d", sessionCountAfter)
		}
	})
}

// TestPropertyForcePasswordResetSetsFlag verifies Property 9:
// Force password reset sets the must_change_password flag.
//
// For any user that has a password (has_password is true), calling
// force-password-reset should result in must_change_password=true on the
// returned user record, and a subsequent fetch should confirm the flag was
// persisted.
func TestPropertyForcePasswordResetSetsFlag(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create a target user with a password.
		passwordHash := rapid.StringMatching(`\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}`).Draw(t, "passwordHash")
		targetUser := User{
			ID:                 uuid.New(),
			Email:              rapid.StringMatching(`[a-z0-9]{3,10}@[a-z]{3,8}\.(com|org|net)`).Draw(t, "email"),
			Name:               rapid.StringMatching(`[A-Za-z ]{3,20}`).Draw(t, "name"),
			Role:               rapid.SampledFrom([]string{"user", "premium", "admin"}).Draw(t, "role"),
			PasswordHash:       &passwordHash,
			MustChangePassword: false,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		_ = userRepo.Create(ctx, &targetUser)

		// Create an acting admin.
		actingAdmin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &actingAdmin)

		// Force password reset.
		updatedUser, err := service.ForcePasswordReset(ctx, actingAdmin.ID, targetUser.ID)
		if err != nil {
			t.Fatalf("ForcePasswordReset returned error: %v", err)
		}

		// Verify the returned user has must_change_password=true.
		if !updatedUser.MustChangePassword {
			t.Fatalf("Expected must_change_password=true in returned user, got false")
		}

		// Fetch the user again to confirm persistence.
		fetchedUser, err := userRepo.FindByID(ctx, targetUser.ID)
		if err != nil {
			t.Fatalf("FindByID returned error: %v", err)
		}
		if fetchedUser == nil {
			t.Fatalf("User not found after force password reset")
			return
		}
		if !fetchedUser.MustChangePassword {
			t.Fatalf("Expected must_change_password=true in fetched user, got false")
		}
	})
}

// TestAdminServiceBusinessRules tests specific business rules enforced by AdminUserService.
func TestAdminServiceBusinessRules(t *testing.T) {
	ctx := context.Background()

	t.Run("self-role-change prevention", func(t *testing.T) {
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create an admin user.
		admin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &admin)

		// Attempt to change own role.
		_, err := service.UpdateRole(ctx, admin.ID, admin.ID, "user")
		if err != ErrCannotChangeSelfRole {
			t.Fatalf("Expected ErrCannotChangeSelfRole, got %v", err)
		}

		// Verify role was not changed.
		fetchedAdmin, _ := userRepo.FindByID(ctx, admin.ID)
		if fetchedAdmin.Role != "admin" {
			t.Fatalf("Admin role changed to %q after self-role-change attempt", fetchedAdmin.Role)
		}
	})

	t.Run("self-disable prevention", func(t *testing.T) {
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create an admin user.
		admin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			Disabled:  false,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &admin)

		// Attempt to disable own account.
		_, err := service.UpdateDisabled(ctx, admin.ID, admin.ID, true)
		if err != ErrCannotDisableSelf {
			t.Fatalf("Expected ErrCannotDisableSelf, got %v", err)
		}

		// Verify disabled status was not changed.
		fetchedAdmin, _ := userRepo.FindByID(ctx, admin.ID)
		if fetchedAdmin.Disabled {
			t.Fatalf("Admin account was disabled after self-disable attempt")
		}
	})

	t.Run("force reset on OAuth-only user rejection", func(t *testing.T) {
		userRepo := newMockUserRepo()
		sessionRepo := newMockSessionRepo()
		service := NewAdminUserService(userRepo, sessionRepo)

		// Create a user with only Google OAuth (no password).
		googleID := "123456789012345"
		oauthUser := User{
			ID:           uuid.New(),
			Email:        "oauth@example.com",
			Name:         "OAuth User",
			Role:         "user",
			GoogleID:     &googleID,
			PasswordHash: nil,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		_ = userRepo.Create(ctx, &oauthUser)

		// Create an admin user.
		admin := User{
			ID:        uuid.New(),
			Email:     "admin@example.com",
			Name:      "Admin User",
			Role:      "admin",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = userRepo.Create(ctx, &admin)

		// Attempt to force password reset on OAuth-only user.
		_, err := service.ForcePasswordReset(ctx, admin.ID, oauthUser.ID)
		if err != ErrNoPasswordAuth {
			t.Fatalf("Expected ErrNoPasswordAuth, got %v", err)
		}
	})
}
