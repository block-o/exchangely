package main

import (
	"bytes"
	"os"
)

func main() {
	fix("tests/integration/lifecycle_test.go")
	fix("internal/worker/backfill_executor_test.go")
}

func fix(path string) {
	b, _ := os.ReadFile(path)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(store, sync, source, nil)"), []byte("NewBackfillExecutor(store, sync, source, nil, nil)"), -1)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(store, sync, nil, nil)"), []byte("NewBackfillExecutor(store, sync, nil, nil, nil)"), -1)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(candleRepo, syncRepo, source, nil)"), []byte("NewBackfillExecutor(candleRepo, syncRepo, source, nil, nil)"), -1)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(candleRepo, syncRepo, &fakeMarketSource{err: context.DeadlineExceeded}, nil)"), []byte("NewBackfillExecutor(candleRepo, syncRepo, &fakeMarketSource{err: context.DeadlineExceeded}, nil, nil)"), -1)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(candleRepo, syncRepo, nil, nil)"), []byte("NewBackfillExecutor(candleRepo, syncRepo, nil, nil, nil)"), -1)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(&fakeCandleStore{}, &fakeSyncWriter{}, nil, nil)"), []byte("NewBackfillExecutor(&fakeCandleStore{}, &fakeSyncWriter{}, nil, nil, nil)"), -1)
	b = bytes.Replace(b, []byte("NewBackfillExecutor(candleRepo, syncRepo, source, publisher)"), []byte("NewBackfillExecutor(candleRepo, syncRepo, source, publisher, nil)"), -1)
	b = bytes.Replace(b, []byte("				err: errors.New(\"source failed\"),\n			}, nil)"), []byte("				err: errors.New(\"source failed\"),\n			}, nil, nil)"), -1)
	os.WriteFile(path, b, 0644)
}
