package lua

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/state"
)

func parseIP(s string) net.IP {
	return net.ParseIP(s)
}

func testConfig() config.Lua {
	return config.Lua{
		Enabled:          true,
		ScriptDir:        "site/lua",
		TimeoutSeconds:   5,
		MaxMemoryMB:      10,
		RateLimitPerPeer: 30,
		RateLimitGlobal:  120,
		HTTPEnabled:      true,
		KVEnabled:        true,
	}
}

func setupEngine(t *testing.T, scripts map[string]string) *Engine {
	t.Helper()
	dir := t.TempDir()
	luaDir := filepath.Join(dir, "site", "lua")
	funcDir := filepath.Join(luaDir, "functions")
	os.MkdirAll(funcDir, 0755)

	for name, src := range scripts {
		var path string
		if filepath.Dir(name) == "functions" {
			path = filepath.Join(funcDir, filepath.Base(name))
		} else {
			path = filepath.Join(luaDir, name)
		}
		os.WriteFile(path, []byte(src), 0644)
	}

	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self-peer-id", func() string { return "TestPeer" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

type capturedReply struct {
	mu      sync.Mutex
	replies []string
}

func (c *capturedReply) send(_ context.Context, _, content string) error {
	c.mu.Lock()
	c.replies = append(c.replies, content)
	c.mu.Unlock()
	return nil
}

func (c *capturedReply) last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.replies) == 0 {
		return ""
	}
	return c.replies[len(c.replies)-1]
}

// ── Engine lifecycle ──

func TestEngineCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig()
	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "peer1", func() string { return "P1" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	luaDir := filepath.Join(dir, "site", "lua")
	funcDir := filepath.Join(luaDir, "functions")
	stateDir := filepath.Join(luaDir, ".state")

	for _, d := range []string{luaDir, funcDir, stateDir} {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			t.Errorf("directory not created: %s", d)
		}
	}
}

func TestEngineLoadsScripts(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"hello.lua": `function handle(args) return "hello " .. args end`,
		"ping.lua":  `function handle(args) return "pong" end`,
	})

	cmds := e.Commands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
}

func TestEngineCommandsSorted(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"zebra.lua": `function handle(args) return "z" end`,
		"alpha.lua": `function handle(args) return "a" end`,
	})

	cmds := e.Commands()
	if cmds[0] != "alpha" || cmds[1] != "zebra" {
		t.Errorf("commands not sorted: %v", cmds)
	}
}

// ── Chat command dispatch ──

func TestDispatchSimpleCommand(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"greet.lua": `function handle(args) return "Hello, " .. args .. "!" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "remote-peer", "!greet World", reply.send)

	if got := reply.last(); got != "Hello, World!" {
		t.Errorf("got %q, want %q", got, "Hello, World!")
	}
}

func TestDispatchNoArgs(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"ping.lua": `function handle(args) return "pong" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!ping", reply.send)

	if got := reply.last(); got != "pong" {
		t.Errorf("got %q, want %q", got, "pong")
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	e := setupEngine(t, map[string]string{})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!nosuchcmd", reply.send)

	if got := reply.last(); got != "Unknown command: nosuchcmd" {
		t.Errorf("got %q", got)
	}
}

func TestDispatchEmptyCommand(t *testing.T) {
	e := setupEngine(t, map[string]string{})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!", reply.send)

	if len(reply.replies) != 0 {
		t.Errorf("expected no reply for empty command, got %v", reply.replies)
	}
}

func TestDispatchCaseInsensitive(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"hello.lua": `function handle(args) return "hi" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!HELLO", reply.send)

	if got := reply.last(); got != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestDispatchNilReturn(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"silent.lua": `function handle(args) end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!silent", reply.send)

	if len(reply.replies) != 0 {
		t.Errorf("expected no reply for nil return, got %v", reply.replies)
	}
}

func TestDispatchNoHandleFunction(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"broken.lua": `x = 42`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!broken", reply.send)

	got := reply.last()
	if got == "" {
		t.Error("expected error reply for missing handle()")
	}
}

// ── Data functions (call) ──

func TestCallFunctionBasic(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"functions/add.lua": `function call(request)
			local a = request.params.a
			local b = request.params.b
			return { sum = a + b }
		end`,
	})

	result, err := e.CallFunction(context.Background(), "peer1", "add", map[string]any{
		"a": 3.0,
		"b": 4.0,
	})
	if err != nil {
		t.Fatal(err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["sum"] != 7.0 {
		t.Errorf("sum = %v, want 7", m["sum"])
	}
}

func TestCallFunctionReturnsArray(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"functions/list.lua": `function call(request)
			return {1, 2, 3}
		end`,
	})

	result, err := e.CallFunction(context.Background(), "peer1", "list", nil)
	if err != nil {
		t.Fatal(err)
	}

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T: %v", result, result)
	}
	if len(arr) != 3 {
		t.Errorf("len = %d, want 3", len(arr))
	}
}

func TestCallFunctionNotFound(t *testing.T) {
	e := setupEngine(t, map[string]string{})

	_, err := e.CallFunction(context.Background(), "peer1", "nope", nil)
	if err == nil {
		t.Error("expected error for missing function")
	}
}

func TestCallFunctionNoCallEntryPoint(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"functions/nocall.lua": `function handle(args) return "chat only" end`,
	})

	_, err := e.CallFunction(context.Background(), "peer1", "nocall", nil)
	if err == nil {
		t.Error("expected error for function without call()")
	}
}

func TestListDataFunctions(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"hello.lua":           `function handle(args) return "hi" end`,
		"functions/score.lua": `--- Scores a quiz\nfunction call(request) return {score=0} end`,
		"functions/calc.lua":  `function call(request) return {result=0} end`,
	})

	funcs := e.ListDataFunctions()
	list, ok := funcs.([]DataFunctionInfo)
	if !ok {
		t.Fatalf("expected []DataFunctionInfo, got %T", funcs)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 data functions, got %d", len(list))
	}
}

// ── Sandbox security ──

func TestSandboxNoRequire(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"evil.lua": `function handle(args) require("os") return "pwned" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!evil", reply.send)

	got := reply.last()
	if got == "pwned" {
		t.Error("require() should be blocked")
	}
}

func TestSandboxNoDofile(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"evil.lua": `function handle(args) dofile("/etc/passwd") return "pwned" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!evil", reply.send)

	got := reply.last()
	if got == "pwned" {
		t.Error("dofile() should be blocked")
	}
}

func TestSandboxNoLoadfile(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"evil.lua": `function handle(args) loadfile("/etc/passwd") return "pwned" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!evil", reply.send)

	got := reply.last()
	if got == "pwned" {
		t.Error("loadfile() should be blocked")
	}
}

func TestSandboxNoOsExecute(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"evil.lua": `function handle(args) return tostring(os.execute("echo pwned")) end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!evil", reply.send)

	got := reply.last()
	if got == "pwned" {
		t.Error("os.execute() should be blocked")
	}
}

func TestSandboxOsTimeAllowed(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"time.lua": `function handle(args) return tostring(os.time()) end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!time", reply.send)

	got := reply.last()
	if got == "" || got[:1] < "1" {
		t.Errorf("os.time() should work, got %q", got)
	}
}

func TestSandboxMathAvailable(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"math.lua": `function handle(args) return tostring(math.floor(3.7)) end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!math", reply.send)

	if got := reply.last(); got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}
}

func TestSandboxStringAvailable(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"str.lua": `function handle(args) return string.upper("hello") end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!str", reply.send)

	if got := reply.last(); got != "HELLO" {
		t.Errorf("got %q, want %q", got, "HELLO")
	}
}

func TestSandboxTableAvailable(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"tbl.lua": `function handle(args)
			local t = {3, 1, 2}
			table.sort(t)
			return tostring(t[1]) .. tostring(t[2]) .. tostring(t[3])
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!tbl", reply.send)

	if got := reply.last(); got != "123" {
		t.Errorf("got %q, want %q", got, "123")
	}
}

// ── goop.peer / goop.self ──

func TestGoopPeerID(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"who.lua": `function handle(args) return goop.peer.id end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "remote-peer-123", "!who", reply.send)

	if got := reply.last(); got != "remote-peer-123" {
		t.Errorf("got %q, want %q", got, "remote-peer-123")
	}
}

func TestGoopSelfID(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"me.lua": `function handle(args) return goop.self.id end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!me", reply.send)

	if got := reply.last(); got != "self-peer-id" {
		t.Errorf("got %q, want %q", got, "self-peer-id")
	}
}

func TestGoopSelfLabel(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"label.lua": `function handle(args) return goop.self.label end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!label", reply.send)

	if got := reply.last(); got != "TestPeer" {
		t.Errorf("got %q, want %q", got, "TestPeer")
	}
}

// ── goop.json ──

func TestGoopJsonEncodeDecode(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"json.lua": `function handle(args)
			local encoded = goop.json.encode({name = "Alice", age = 30})
			local decoded = goop.json.decode(encoded)
			return decoded.name .. ":" .. tostring(decoded.age)
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!json", reply.send)

	if got := reply.last(); got != "Alice:30" {
		t.Errorf("got %q, want %q", got, "Alice:30")
	}
}

func TestGoopJsonDecodeInvalid(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"badjson.lua": `function handle(args)
			local val, err = goop.json.decode("not json")
			if err then return "error: " .. err end
			return "unexpected success"
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!badjson", reply.send)

	got := reply.last()
	if len(got) < 6 || got[:6] != "error:" {
		t.Errorf("expected error message, got %q", got)
	}
}

// ── goop.kv ──

func TestGoopKVSetGetDel(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"kv.lua": `function handle(args)
			goop.kv.set("mykey", "myval")
			local val = goop.kv.get("mykey")
			goop.kv.del("mykey")
			local gone = goop.kv.get("mykey")
			if gone == nil then
				return val .. ":deleted"
			end
			return val .. ":not_deleted"
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!kv", reply.send)

	if got := reply.last(); got != "myval:deleted" {
		t.Errorf("got %q, want %q", got, "myval:deleted")
	}
}

func TestGoopKVGetMissing(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"kvmiss.lua": `function handle(args)
			local val = goop.kv.get("nonexistent")
			if val == nil then return "nil" end
			return tostring(val)
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!kvmiss", reply.send)

	if got := reply.last(); got != "nil" {
		t.Errorf("got %q, want %q", got, "nil")
	}
}

func TestGoopKVPersistsAcrossInvocations(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"kvset.lua": `function handle(args) goop.kv.set("counter", args) return "set" end`,
		"kvget.lua": `function handle(args) return goop.kv.get("counter") or "nil" end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!kvset 42", reply.send)

	// KV is per-script, so kvget won't see kvset's data
	e.DispatchCommand(context.Background(), "peer1", "!kvget", reply.send)
	if got := reply.last(); got != "nil" {
		t.Errorf("KV should be per-script, got %q", got)
	}

	// But same script sees its own data
	e2 := setupEngine(t, map[string]string{
		"persist.lua": `function handle(args)
			if args ~= "" then
				goop.kv.set("val", args)
				return "stored"
			end
			return goop.kv.get("val") or "empty"
		end`,
	})

	reply2 := &capturedReply{}
	e2.DispatchCommand(context.Background(), "peer1", "!persist hello", reply2.send)
	e2.DispatchCommand(context.Background(), "peer1", "!persist", reply2.send)

	if got := reply2.last(); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// ── goop.commands ──

func TestGoopCommands(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"alpha.lua": `function handle(args) return "a" end`,
		"beta.lua":  `function handle(args) return "b" end`,
		"cmds.lua":  `function handle(args)
			local c = goop.commands()
			return table.concat(c, ",")
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!cmds", reply.send)

	got := reply.last()
	if got != "alpha,beta,cmds" {
		t.Errorf("got %q, want %q", got, "alpha,beta,cmds")
	}
}

// ── Script metadata ──

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"--- A cool command\nfunction handle(args) end", "A cool command"},
		{"--- @rate_limit 10\n--- My script\nfunction handle(args) end", "My script"},
		{"function handle(args) end", ""},
		{"--- \nfunction handle(args) end", ""},
	}

	for _, tt := range tests {
		got := extractDescription(tt.source)
		if got != tt.want {
			t.Errorf("extractDescription(%q) = %q, want %q", tt.source[:20], got, tt.want)
		}
	}
}

func TestExtractRateLimit(t *testing.T) {
	tests := []struct {
		source string
		want   int
	}{
		{"--- @rate_limit 10\nfunction handle(args) end", 10},
		{"--- @rate_limit 0\nfunction handle(args) end", 0},
		{"--- A command\nfunction handle(args) end", -1},
		{"function handle(args) end", -1},
	}

	for _, tt := range tests {
		got := extractRateLimit(tt.source)
		if got != tt.want {
			t.Errorf("extractRateLimit(%q) = %d, want %d", tt.source[:20], got, tt.want)
		}
	}
}

func TestDetectEntryPoint(t *testing.T) {
	tests := []struct {
		source string
		fn     string
		want   bool
	}{
		{"function handle(args) end", "handle", true},
		{"function call(request) end", "call", true},
		{"function call (request) end", "call", true},
		{"function handle(args) end", "call", false},
		{"local x = 42", "handle", false},
	}

	for _, tt := range tests {
		got := detectEntryPoint(tt.source, tt.fn)
		if got != tt.want {
			t.Errorf("detectEntryPoint(%q, %q) = %v, want %v", tt.source, tt.fn, got, tt.want)
		}
	}
}

// ── Rate limiting ──

func TestRateLimitPerPeer(t *testing.T) {
	rl := newRateLimiter(3, 1000)

	for i := 0; i < 3; i++ {
		if !rl.AllowFunc("peer1", "cmd", -1) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if rl.AllowFunc("peer1", "cmd", -1) {
		t.Error("4th request should be rate limited")
	}

	// Different peer should still be allowed
	if !rl.AllowFunc("peer2", "cmd", -1) {
		t.Error("different peer should be allowed")
	}
}

func TestRateLimitGlobal(t *testing.T) {
	rl := newRateLimiter(100, 2)

	rl.AllowFunc("peer1", "cmd", -1)
	rl.AllowFunc("peer2", "cmd", -1)

	if rl.AllowFunc("peer3", "cmd", -1) {
		t.Error("global limit should be enforced")
	}
}

func TestRateLimitCustomPerFunction(t *testing.T) {
	rl := newRateLimiter(100, 1000)

	if !rl.AllowFunc("peer1", "limited", 1) {
		t.Error("first call should be allowed")
	}
	if rl.AllowFunc("peer1", "limited", 1) {
		t.Error("second call with limit=1 should be rejected")
	}
}

func TestRateLimitZeroDisables(t *testing.T) {
	rl := newRateLimiter(1, 1000)

	for i := 0; i < 100; i++ {
		if !rl.AllowFunc("peer1", "unlimited", 0) {
			t.Fatalf("unlimited function blocked at request %d", i+1)
		}
	}
}

// ── Timeout ──

func TestScriptTimeout(t *testing.T) {
	dir := t.TempDir()
	luaDir := filepath.Join(dir, "site", "lua")
	funcDir := filepath.Join(luaDir, "functions")
	os.MkdirAll(funcDir, 0755)
	os.WriteFile(filepath.Join(luaDir, "slow.lua"), []byte(`function handle(args) while true do end end`), 0644)

	cfg := testConfig()
	cfg.TimeoutSeconds = 1

	peers := state.NewPeerTable()
	e, err := NewEngine(cfg, dir, "self", func() string { return "S" }, peers)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	reply := &capturedReply{}
	start := time.Now()
	e.DispatchCommand(context.Background(), "peer1", "!slow", reply.send)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}

	got := reply.last()
	if got == "" {
		t.Error("expected timeout error reply")
	}
}

// ── Reply truncation ──

func TestReplyTruncation(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"big.lua": `function handle(args)
			return string.rep("x", 10000)
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!big", reply.send)

	got := reply.last()
	if len(got) > maxReplyBytes+50 {
		t.Errorf("reply not truncated: %d bytes", len(got))
	}
}

// ── Compile errors ──

func TestCompileErrorDoesNotCrash(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"bad.lua": `function this is not valid lua`,
	})

	cmds := e.Commands()
	for _, c := range cmds {
		if c == "bad" {
			t.Error("broken script should not be loaded")
		}
	}
}

// ── Go<->Lua type conversion ──

func TestGoToLuaTypes(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"functions/types.lua": `function call(request)
			local p = request.params
			return {
				str = p.str,
				num = p.num,
				bool_val = p.bool_val,
				arr_len = #p.arr
			}
		end`,
	})

	result, err := e.CallFunction(context.Background(), "peer1", "types", map[string]any{
		"str":      "hello",
		"num":      42.0,
		"bool_val": true,
		"arr":      []interface{}{1.0, 2.0, 3.0},
	})
	if err != nil {
		t.Fatal(err)
	}

	m := result.(map[string]interface{})
	if m["str"] != "hello" {
		t.Errorf("str = %v", m["str"])
	}
	if m["num"] != 42.0 {
		t.Errorf("num = %v", m["num"])
	}
	if m["bool_val"] != true {
		t.Errorf("bool_val = %v", m["bool_val"])
	}
	if m["arr_len"] != 3.0 {
		t.Errorf("arr_len = %v", m["arr_len"])
	}
}

// ── SSRF protection ──

func TestSSRFBlocksLoopback(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"ssrf.lua": `function handle(args)
			local body, err = goop.http.get("http://127.0.0.1:1234/secret")
			if err then return "blocked: " .. err end
			return "leaked: " .. body
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!ssrf", reply.send)

	got := reply.last()
	if len(got) > 8 && got[:7] == "leaked:" {
		t.Error("SSRF to loopback should be blocked")
	}
}

func TestSSRFBlocksPrivate(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"ssrf.lua": `function handle(args)
			local body, err = goop.http.get("http://192.168.1.1/admin")
			if err then return "blocked: " .. err end
			return "leaked: " .. body
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!ssrf", reply.send)

	got := reply.last()
	if len(got) > 8 && got[:7] == "leaked:" {
		t.Error("SSRF to private IP should be blocked")
	}
}

func TestHTTPRequestLimit(t *testing.T) {
	e := setupEngine(t, map[string]string{
		"flood.lua": `function handle(args)
			for i = 1, 5 do
				local _, err = goop.http.get("http://example.com")
				if err then return "limited at " .. tostring(i) end
			end
			return "all passed"
		end`,
	})

	reply := &capturedReply{}
	e.DispatchCommand(context.Background(), "peer1", "!flood", reply.send)

	got := reply.last()
	if got == "all passed" {
		t.Error("HTTP request limit should be enforced at 3")
	}
}

// ── checkIP unit tests ──

func TestCheckIPBlocksPrivateAddresses(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		ip := parseIP(tt.ip)
		err := checkIP(ip)
		if tt.blocked && err == nil {
			t.Errorf("checkIP(%s) should block", tt.ip)
		}
		if !tt.blocked && err != nil {
			t.Errorf("checkIP(%s) should allow", tt.ip)
		}
	}
}
