// Serves the embedded phone remote bundle (apps/remote-web).

package remote

import (
	"bytes"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// contentTypes pins the types the embedded bundle needs so nothing depends
// on the host's mime database; unknown extensions are served as octet-stream
// and never sniffed.
var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".map":         "application/json; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".webp":        "image/webp",
	".ico":         "image/x-icon",
	".woff2":       "font/woff2",
	".woff":        "font/woff",
	".txt":         "text/plain; charset=utf-8",
}

// serveStatic serves the embedded phone remote: "/" maps to index.html,
// directories are never listed, and any traversal attempt is a 404.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "static assets accept GET and HEAD only")
		return
	}
	if strings.Contains(r.URL.Path, "..") || strings.Contains(r.URL.Path, "\\") {
		writeError(w, http.StatusNotFound, "not_found", "no such asset")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		writeError(w, http.StatusNotFound, "not_found", "no such asset")
		return
	}
	root := s.static
	if rest, ok := strings.CutPrefix(name, "themes/"); ok && s.opts.ThemeAssets != nil {
		// Theme art for phones (internal/themes.Registry.Assets: image files of
		// loaded themes and the built-in ornaments only).
		root, name = s.opts.ThemeAssets, rest
	}
	f, err := root.Open(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such asset")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		writeError(w, http.StatusNotFound, "not_found", "no such asset")
		return
	}
	ext := strings.ToLower(path.Ext(name))
	ctype, ok := contentTypes[ext]
	if !ok {
		ctype = mime.TypeByExtension(ext)
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		b, err := io.ReadAll(f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "asset read failed")
			return
		}
		rs = bytes.NewReader(b)
	}
	http.ServeContent(w, r, name, time.Time{}, rs)
}
