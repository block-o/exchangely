import { apiGet } from "./client";
import type { HealthResponse } from "../types/api";

export function fetchHealth() {
  return apiGet<HealthResponse>("/health");
}
