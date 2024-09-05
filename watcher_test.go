package frankenphp_test

import (
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// we have to wait a few milliseconds for the watcher debounce to take effect
const pollingTime = 150
const minTimesToPollForChanges = 5
const maxTimesToPollForChanges = 100 // we will poll a maximum of 100x150ms = 15s

func TestWorkerShouldReloadOnMatchingPattern(t *testing.T) {
	const filePattern = "./testdata/**/*.txt"

	runTest(t, func(handler func(http.ResponseWriter, *http.Request), _ *httptest.Server, i int) {
		// first we verify that the worker is working correctly
		body := fetchBody("GET", "http://example.com/worker-with-watcher.php", handler)
		assert.Equal(t, "requests:1", body)

		// now we verify that updating a .txt file does not cause a reload
		requestBodyHasReset := pollForWorkerReset(t, handler, maxTimesToPollForChanges)

		assert.True(t, requestBodyHasReset)
	}, &testOptions{nbParrallelRequests: 1, nbWorkers: 1, workerScript: "worker-with-watcher.php", watch: filePattern})
}

func TestWorkerShouldNotReloadOnNonMatchingPattern(t *testing.T) {
	const filePattern = "./testdata/**/*.php"

	runTest(t, func(handler func(http.ResponseWriter, *http.Request), _ *httptest.Server, i int) {
		// first we verify that the worker is working correctly
		body := fetchBody("GET", "http://example.com/worker-with-watcher.php", handler)
		assert.Equal(t, "requests:1", body)

		// now we verify that updating a .json file does not cause a reload
		requestBodyHasReset := pollForWorkerReset(t, handler, minTimesToPollForChanges)

		assert.False(t, requestBodyHasReset)

	}, &testOptions{nbParrallelRequests: 1, nbWorkers: 1, workerScript: "worker-with-watcher.php", watch: filePattern})
}

func fetchBody(method string, url string, handler func(http.ResponseWriter, *http.Request)) string {
	req := httptest.NewRequest(method, url, nil)
	w := httptest.NewRecorder()
	handler(w, req)
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	return string(body)
}

func pollForWorkerReset(t *testing.T, handler func(http.ResponseWriter, *http.Request), limit int) bool {
	for i := 0; i < limit; i++ {
		updateTestFile("./testdata/files/test.txt", "updated", t)
		time.Sleep(pollingTime * time.Millisecond)
		body := fetchBody("GET", "http://example.com/worker-with-watcher.php", handler)
		if body == "requests:1" {
			return true
		}
	}
	return false
}

func updateTestFile(fileName string, content string, t *testing.T) {
	absFileName, err := filepath.Abs(fileName)
	assert.NoError(t, err)
	dirName := filepath.Dir(absFileName)
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		err = os.MkdirAll(dirName, 0700)
		assert.NoError(t, err)
	}
	bytes := []byte(content)
	err = os.WriteFile(absFileName, bytes, 0644)
	assert.NoError(t, err)
}
