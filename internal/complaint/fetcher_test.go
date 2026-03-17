package complaint

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cmon/internal/config"
	"cmon/internal/session"
	"cmon/internal/storage"
)

func withTempCWD(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func TestFetchAllFailsOnIncompletePagination(t *testing.T) {
	withTempCWD(t)

	stor := storage.New()
	t.Cleanup(func() {
		_ = stor.Close()
	})

	if err := stor.SaveMultiple([]storage.Record{{
		ComplaintID: "CMP-1",
		APIID:       "API-1",
	}}); err != nil {
		t.Fatalf("save complaint: %v", err)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page1":
			fmt.Fprintf(w, `
				<html>
					<body>
						<table id="dataTable">
							<tbody>
								<tr>
									<td><a onclick="openModelData(1)">CMP-1</a></td>
								</tr>
							</tbody>
						</table>
						<ul class="pagination">
							<li><a class="page-link" href="%s/page2">Next</a></li>
						</ul>
					</body>
				</html>
			`, server.URL)
		case "/page2":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sc, err := session.New()
	if err != nil {
		t.Fatalf("new session client: %v", err)
	}

	fetcher := New(sc, stor, nil, nil, &config.Config{
		MaxPages:       5,
		WorkerPoolSize: 1,
	}, nil)

	_, err = fetcher.FetchAll(server.URL + "/page1")
	if err == nil {
		t.Fatal("expected FetchAll to fail when pagination cannot be completed")
	}
	if !strings.Contains(err.Error(), "failed to fetch page 2") {
		t.Fatalf("expected pagination error, got %v", err)
	}
}
