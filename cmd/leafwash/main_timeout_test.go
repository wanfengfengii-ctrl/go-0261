package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const modelChildEnv = "LEAFWASH_MODEL_TIMEOUT_TEST_CHILD"

func TestModel_LeafWashExecutableTimeouts(t *testing.T) {
	var (
		inspectOnce sync.Once
		inspection  entrypointInspection
		inspectErr  error

		startOnce sync.Once
		started   *modelServer
		startErr  error
	)

	inspect := func() (entrypointInspection, error) {
		inspectOnce.Do(func() {
			inspection, inspectErr = inspectEntrypointServerTimeouts()
		})
		return inspection, inspectErr
	}

	start := func() (*modelServer, error) {
		startOnce.Do(func() {
			started, startErr = startModelServer(t)
		})
		return started, startErr
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "serve_child_process",
			run: func(t *testing.T) {
				if os.Getenv(modelChildEnv) != "1" {
					t.Skip("child process only")
				}
				main()
				t.Fatal("leafwash main returned")
			},
		},
		{
			name: "entrypoint_uses_explicit_http_server_timeouts",
			run: func(t *testing.T) {
				got, err := inspect()
				if err != nil {
					t.Fatal(err)
				}
				if !got.hasHTTPServer {
					t.Fatal("cmd/leafwash must construct an explicit http.Server")
				}
				if got.usesPackageListenAndServe {
					t.Fatal("cmd/leafwash must serve through its configured http.Server, not http.ListenAndServe")
				}
				if len(got.missingTimeouts) > 0 {
					t.Fatalf("http.Server missing non-zero timeout fields: %s", strings.Join(got.missingTimeouts, ", "))
				}
			},
		},
		{
			name: "env_addr_health_frontend_and_json_api_still_work",
			run: func(t *testing.T) {
				server, err := start()
				if err != nil {
					t.Fatal(err)
				}

				client := &http.Client{Timeout: 2 * time.Second}
				assertGET(t, client, server.baseURL+"/healthz", http.StatusOK, `"status":"ok"`)
				assertGET(t, client, server.baseURL+"/", http.StatusOK, "LeafWash")

				body := `{"task_id":"TASK-MODEL-TIMEOUT","base_lot_id":"BL-2026-001","seal_id":"SEAL-001",
					"precool_lot":"PC-001","cut_line_id":"CUT-3","wash_tank_id":"TANK-A",
					"formula_id":"F-100","formula_revision":3,
					"sample_times":[0,300,600],"blind_codes":["BLIND-01","BLIND-02","BLIND-03"],
					"atp_points":["ATP-1","ATP-2","ATP-3"],
					"plate_wells":["WELL-A1","WELL-A2","WELL-B1"],
					"drain_slots":["DRAIN-1"],"reviewers":["P-2001","P-2002"]}`
				resp, err := client.Post(server.baseURL+"/api/tasks/lock", "application/json", strings.NewReader(body))
				if err != nil {
					t.Fatalf("post lock: %v", err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusCreated {
					b, _ := io.ReadAll(resp.Body)
					t.Fatalf("lock status = %d, want %d; body=%s", resp.StatusCode, http.StatusCreated, string(b))
				}
				var decoded struct {
					TaskID string `json:"task_id"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
					t.Fatalf("decode lock response: %v", err)
				}
				if decoded.TaskID != "TASK-MODEL-TIMEOUT" {
					t.Fatalf("lock task_id = %q, want TASK-MODEL-TIMEOUT", decoded.TaskID)
				}
			},
		},
		{
			name: "incomplete_headers_are_closed_by_read_header_timeout",
			run: func(t *testing.T) {
				server, err := start()
				if err != nil {
					t.Fatal(err)
				}

				wait := 7 * time.Second
				if got, err := inspect(); err == nil {
					if configured := got.timeoutValues["ReadHeaderTimeout"]; configured > 0 && configured < 15*time.Second {
						wait = configured + time.Second
					}
				}

				conn, err := net.DialTimeout("tcp", server.addr, time.Second)
				if err != nil {
					t.Fatalf("dial server: %v", err)
				}
				defer conn.Close()

				partialHeader := "GET /healthz HTTP/1.1\r\nHost: leafwash.local\r\nUser-Agent: model-timeout-test\r\n"
				if _, err := io.WriteString(conn, partialHeader); err != nil {
					t.Fatalf("write partial header: %v", err)
				}

				if err := conn.SetReadDeadline(time.Now().Add(wait)); err != nil {
					t.Fatalf("set read deadline: %v", err)
				}
				var b [1]byte
				_, err = conn.Read(b[:])
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					t.Fatalf("server kept an incomplete HTTP request header open for %s", wait)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

type entrypointInspection struct {
	hasHTTPServer             bool
	usesPackageListenAndServe bool
	missingTimeouts           []string
	timeoutValues             map[string]time.Duration
}

func inspectEntrypointServerTimeouts() (entrypointInspection, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		return entrypointInspection{}, err
	}
	pkg, ok := pkgs["main"]
	if !ok {
		return entrypointInspection{}, fmt.Errorf("main package not found")
	}

	consts := collectDurationValues(pkg)
	fields := map[string]bool{
		"ReadHeaderTimeout": false,
		"ReadTimeout":       false,
		"WriteTimeout":      false,
		"IdleTimeout":       false,
	}
	values := make(map[string]time.Duration)
	serverVars := make(map[string]bool)
	got := entrypointInspection{timeoutValues: values}

	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				if isHTTPServerType(node.Type) {
					got.hasHTTPServer = true
					recordTimeoutFields(node.Elts, fields, values, consts)
				}
			case *ast.AssignStmt:
				for i, rhs := range node.Rhs {
					if isHTTPServerExpr(rhs) && i < len(node.Lhs) {
						if id, ok := node.Lhs[i].(*ast.Ident); ok {
							serverVars[id.Name] = true
						}
					}
				}
				recordTimeoutAssignments(node.Lhs, node.Rhs, serverVars, fields, values, consts)
			case *ast.ValueSpec:
				for i, value := range node.Values {
					if isHTTPServerExpr(value) && i < len(node.Names) {
						serverVars[node.Names[i].Name] = true
					}
				}
			case *ast.CallExpr:
				if isHTTPListenAndServe(node.Fun) {
					got.usesPackageListenAndServe = true
				}
			}
			return true
		})
	}

	for _, field := range sortedKeys(fields) {
		if !fields[field] {
			got.missingTimeouts = append(got.missingTimeouts, field)
		}
	}
	return got, nil
}

func collectDurationValues(pkg *ast.Package) map[string]time.Duration {
	consts := make(map[string]time.Duration)
	for changed := true; changed; {
		changed = false
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
					continue
				}
				for _, spec := range gen.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						if _, exists := consts[name.Name]; exists || i >= len(vs.Values) {
							continue
						}
						if d, ok := evalDuration(vs.Values[i], consts); ok {
							consts[name.Name] = d
							changed = true
						}
					}
				}
			}
		}
	}
	return consts
}

func recordTimeoutFields(elts []ast.Expr, fields map[string]bool, values map[string]time.Duration, consts map[string]time.Duration) {
	for _, elt := range elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if _, wanted := fields[id.Name]; wanted && !isZeroish(kv.Value) {
			fields[id.Name] = true
			if d, ok := evalDuration(kv.Value, consts); ok {
				values[id.Name] = d
			}
		}
	}
}

func recordTimeoutAssignments(lhs, rhs []ast.Expr, serverVars map[string]bool, fields map[string]bool, values map[string]time.Duration, consts map[string]time.Duration) {
	for i, left := range lhs {
		sel, ok := left.(*ast.SelectorExpr)
		if !ok || i >= len(rhs) {
			continue
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || !serverVars[id.Name] {
			continue
		}
		if _, wanted := fields[sel.Sel.Name]; wanted && !isZeroish(rhs[i]) {
			fields[sel.Sel.Name] = true
			if d, ok := evalDuration(rhs[i], consts); ok {
				values[sel.Sel.Name] = d
			}
		}
	}
}

func isHTTPServerExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return isHTTPServerType(e.Type)
	case *ast.UnaryExpr:
		return e.Op == token.AND && isHTTPServerExpr(e.X)
	default:
		return false
	}
}

func isHTTPServerType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Server" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "http"
}

func isHTTPListenAndServe(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ListenAndServe" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "http"
}

func isZeroish(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return strings.Trim(e.Value, "`\"") == "0"
	case *ast.Ident:
		return e.Name == "0" || e.Name == "nil"
	case *ast.ParenExpr:
		return isZeroish(e.X)
	default:
		return false
	}
}

func evalDuration(expr ast.Expr, consts map[string]time.Duration) (time.Duration, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		d, ok := consts[e.Name]
		return d, ok
	case *ast.ParenExpr:
		return evalDuration(e.X, consts)
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok && id.Name == "time" {
			switch e.Sel.Name {
			case "Nanosecond":
				return time.Nanosecond, true
			case "Microsecond":
				return time.Microsecond, true
			case "Millisecond":
				return time.Millisecond, true
			case "Second":
				return time.Second, true
			case "Minute":
				return time.Minute, true
			case "Hour":
				return time.Hour, true
			}
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.MUL:
			if n, ok := evalScalar(e.X); ok {
				if d, ok := evalDuration(e.Y, consts); ok {
					return time.Duration(n) * d, true
				}
			}
			if n, ok := evalScalar(e.Y); ok {
				if d, ok := evalDuration(e.X, consts); ok {
					return time.Duration(n) * d, true
				}
			}
		case token.ADD:
			left, okLeft := evalDuration(e.X, consts)
			right, okRight := evalDuration(e.Y, consts)
			if okLeft && okRight {
				return left + right, true
			}
		}
	}
	return 0, false
}

func evalScalar(expr ast.Expr) (int64, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.ParseInt(e.Value, 0, 64)
		return n, err == nil
	case *ast.ParenExpr:
		return evalScalar(e.X)
	default:
		return 0, false
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type modelServer struct {
	addr    string
	baseURL string
	cmd     *exec.Cmd
	done    chan error
	output  lockedBuffer
	exited  bool
	waitErr error
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func startModelServer(t *testing.T) (*modelServer, error) {
	t.Helper()
	addr, err := freeTCPAddr()
	if err != nil {
		return nil, err
	}

	server := &modelServer{
		addr:    addr,
		baseURL: "http://" + addr,
		done:    make(chan error, 1),
	}
	server.cmd = exec.Command(os.Args[0], "-test.run=^TestModel_LeafWashExecutableTimeouts$/^serve_child_process$")
	server.cmd.Env = append(os.Environ(),
		modelChildEnv+"=1",
		"LEAFWASH_ADDR="+addr,
		"LEAFWASH_DATA="+filepath.Join(t.TempDir(), "leafwash.db"),
	)
	server.cmd.Stdout = &server.output
	server.cmd.Stderr = &server.output
	if err := server.cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		server.done <- server.cmd.Wait()
	}()
	t.Cleanup(server.stop)

	if err := server.waitReady(); err != nil {
		server.stop()
		return nil, err
	}
	return server, nil
}

func freeTCPAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}

func (s *modelServer) waitReady() error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if err, ok := s.pollExit(); ok {
			return fmt.Errorf("server exited before readiness: %v\n%s", err, s.output.String())
		}
		resp, err := client.Get(s.baseURL + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready at %s\n%s", s.baseURL, s.output.String())
}

func (s *modelServer) pollExit() (error, bool) {
	if s.exited {
		return s.waitErr, true
	}
	select {
	case err := <-s.done:
		s.exited = true
		s.waitErr = err
		return err, true
	default:
		return nil, false
	}
}

func (s *modelServer) stop() {
	if s.exited {
		return
	}
	_ = s.cmd.Process.Kill()
	s.waitErr = <-s.done
	s.exited = true
}

func assertGET(t *testing.T, client *http.Client, url string, status int, contains string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get %s status = %d, want %d; body=%s", url, resp.StatusCode, status, string(b))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if !strings.Contains(string(b), contains) {
		t.Fatalf("get %s body does not contain %q", url, contains)
	}
}
