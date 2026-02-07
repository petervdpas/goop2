package lua

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/config"
	"github.com/petervdpas/goop2/internal/state"
	"github.com/petervdpas/goop2/internal/storage"
	"github.com/petervdpas/goop2/internal/util"

	"github.com/fsnotify/fsnotify"
	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

const maxReplyBytes = 4096

// DirectSender sends a direct message to a peer.
type DirectSender interface {
	SendDirect(ctx context.Context, toPeerID, content string) error
}

// SenderFunc adapts a plain function to the DirectSender interface.
type SenderFunc func(ctx context.Context, toPeerID, content string) error

func (f SenderFunc) SendDirect(ctx context.Context, toPeerID, content string) error {
	return f(ctx, toPeerID, content)
}

// scriptMeta holds compiled script along with Phase 2 metadata.
type scriptMeta struct {
	proto       *lua.FunctionProto
	description string // from leading --- comment
	hasCall     bool   // script defines call() entry point
	isFunction  bool   // true if loaded from functions/ subdirectory
	rateLimit   int    // -1 = use default, 0 = unlimited, N>0 = custom per-peer-per-minute
}

// DataFunctionInfo describes a Lua data function for lua-list responses.
type DataFunctionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Engine manages Lua scripts, hot reload, and command dispatch.
type Engine struct {
	mu           sync.RWMutex
	scripts      map[string]*scriptMeta // command name -> compiled script + metadata
	cfg          config.Lua
	scriptDir    string // site/lua/ — chat scripts
	functionsDir string // site/lua/functions/ — data functions
	kv           *kvStore
	db           *storage.DB
	watcher      *fsnotify.Watcher
	limiter      *rateLimiter
	selfID       string
	selfLabel    func() string
	peers        *state.PeerTable
	closed       chan struct{}
}

// NewEngine creates and starts a Lua scripting engine.
func NewEngine(cfg config.Lua, peerDir string, selfID string, selfLabel func() string, peers *state.PeerTable) (*Engine, error) {
	scriptDir := util.ResolvePath(peerDir, cfg.ScriptDir)
	functionsDir := filepath.Join(scriptDir, "functions")
	stateDir := filepath.Join(scriptDir, ".state")

	for _, dir := range []string{scriptDir, functionsDir, stateDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	e := &Engine{
		scripts:      make(map[string]*scriptMeta),
		cfg:          cfg,
		scriptDir:    scriptDir,
		functionsDir: functionsDir,
		kv:           newKVStore(stateDir),
		watcher:      watcher,
		limiter:      newRateLimiter(cfg.RateLimitPerPeer, cfg.RateLimitGlobal),
		selfID:       selfID,
		selfLabel:    selfLabel,
		peers:        peers,
		closed:       make(chan struct{}),
	}

	// Scan chat scripts in scriptDir
	e.scanDir(scriptDir, false)

	// Scan data functions in functionsDir
	e.scanDir(functionsDir, true)

	// Watch both directories
	if err := watcher.Add(scriptDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch script dir: %w", err)
	}
	if err := watcher.Add(functionsDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch functions dir: %w", err)
	}

	go e.watchLoop()

	log.Printf("LUA: engine started, %d script(s) loaded from %s", len(e.scripts), scriptDir)
	return e, nil
}

func (e *Engine) scanDir(dir string, isFunction bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".lua")
		if err := e.compileScriptAs(name, filepath.Join(dir, entry.Name()), isFunction); err != nil {
			log.Printf("LUA: failed to compile %s: %v", entry.Name(), err)
		}
	}
}

func (e *Engine) compileScript(name, path string) error {
	// Detect if this file is in the functions/ subdirectory
	isFunction := strings.HasPrefix(filepath.Dir(path), e.functionsDir)
	return e.compileScriptAs(name, path, isFunction)
}

func (e *Engine) compileScriptAs(name, path string, isFunction bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	source := string(data)

	chunk, err := parse.Parse(strings.NewReader(source), name)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	meta := &scriptMeta{
		proto:       proto,
		description: extractDescription(source),
		hasCall:     detectEntryPoint(source, "call"),
		isFunction:  isFunction,
		rateLimit:   extractRateLimit(source),
	}

	e.mu.Lock()
	e.scripts[name] = meta
	e.mu.Unlock()

	kind := "chat"
	if isFunction {
		kind = "function"
	}
	log.Printf("LUA: compiled %s script %q (call=%v)", kind, name, meta.hasCall)
	return nil
}

// extractDescription returns the first --- comment from a script source.
func extractDescription(source string) string {
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "---") {
			desc := strings.TrimSpace(strings.TrimPrefix(line, "---"))
			if !strings.HasPrefix(desc, "@") {
				return desc
			}
			continue
		}
		break
	}
	return ""
}

var rateLimitRe = regexp.MustCompile(`^---\s*@rate_limit\s+(\d+)`)

// extractRateLimit parses a --- @rate_limit N annotation from the leading comment block.
// Returns -1 (use default), 0 (unlimited), or N>0 (custom limit).
func extractRateLimit(source string) int {
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "---") {
			break
		}
		if m := rateLimitRe.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err == nil {
				return n
			}
		}
	}
	return -1
}

// detectEntryPoint checks if a script defines a given function name.
func detectEntryPoint(source, funcName string) bool {
	// Match "function <name>(" with optional whitespace
	pattern := "function " + funcName + "("
	patternAlt := "function " + funcName + " ("
	return strings.Contains(source, pattern) || strings.Contains(source, patternAlt)
}

func (e *Engine) removeScript(name string) {
	e.mu.Lock()
	delete(e.scripts, name)
	e.mu.Unlock()
	log.Printf("LUA: removed script %q", name)
}

func (e *Engine) watchLoop() {
	for {
		select {
		case <-e.closed:
			return
		case event, ok := <-e.watcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(event.Name, ".lua") {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(event.Name), ".lua")

			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				if err := e.compileScript(name, event.Name); err != nil {
					log.Printf("LUA: hot reload failed for %s: %v", name, err)
				}
			}
			if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				e.removeScript(name)
			}
		case err, ok := <-e.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("LUA: watcher error: %v", err)
		}
	}
}

// Dispatch handles a command message. It parses the command, checks rate limits,
// looks up the script, executes it, and sends the reply.
func (e *Engine) Dispatch(ctx context.Context, fromPeerID, content string, sender DirectSender) {
	// Parse "!command args"
	body := strings.TrimPrefix(content, "!")
	parts := strings.SplitN(body, " ", 2)
	cmdName := strings.ToLower(strings.TrimSpace(parts[0]))
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	if cmdName == "" {
		return
	}

	// Lookup script
	e.mu.RLock()
	meta, ok := e.scripts[cmdName]
	e.mu.RUnlock()

	if !ok {
		reply := fmt.Sprintf("Unknown command: %s", cmdName)
		_ = sender.SendDirect(ctx, fromPeerID, reply)
		return
	}

	// Rate limit (per-function)
	if !e.limiter.AllowFunc(fromPeerID, cmdName, meta.rateLimit) {
		_ = sender.SendDirect(ctx, fromPeerID, "Rate limit exceeded. Try again later.")
		return
	}

	// Resolve peer label
	peerLabel := fromPeerID
	if fromPeerID == e.selfID {
		peerLabel = e.selfLabel()
	} else if sp, ok := e.peers.Get(fromPeerID); ok {
		peerLabel = sp.Content
	}

	// Execute with timeout
	timeout := time.Duration(e.cfg.TimeoutSeconds) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	inv := &invocationCtx{
		ctx:        execCtx,
		scriptName: cmdName,
		peerID:     fromPeerID,
		peerLabel:  peerLabel,
		selfID:     e.selfID,
		selfLabel:  e.selfLabel(),
	}

	result, err := e.executeScript(execCtx, inv, meta.proto, args)
	if err != nil {
		log.Printf("LUA: error executing %s: %v", cmdName, err)
		result = fmt.Sprintf("Script error: %v", err)
	}

	// Truncate reply
	if len(result) > maxReplyBytes {
		result = result[:maxReplyBytes] + "... (truncated)"
	}

	if result != "" {
		if err := sender.SendDirect(ctx, fromPeerID, result); err != nil {
			log.Printf("LUA: failed to send reply to %s: %v", fromPeerID, err)
		}
	}
}

func (e *Engine) executeScript(ctx context.Context, inv *invocationCtx, proto *lua.FunctionProto, args string) (string, error) {
	L := newSandboxedVM(inv, e.kv, e)

	var closeOnce sync.Once
	closeL := func() { closeOnce.Do(func() { L.Close() }) }
	defer closeL()

	// Load compiled proto
	lfunc := L.NewFunctionFromProto(proto)
	L.Push(lfunc)
	if err := L.PCall(0, lua.MultRet, nil); err != nil {
		return "", fmt.Errorf("load script: %w", err)
	}

	// Call handle(args)
	handleFn := L.GetGlobal("handle")
	if handleFn == lua.LNil {
		return "", fmt.Errorf("script has no handle() function")
	}

	memMon := newMemoryMonitor(e.cfg.MaxMemoryMB)
	stopMon := memMon.watch(ctx, L, inv.scriptName)

	// Run in goroutine so we can kill on timeout
	type result struct {
		val string
		err error
	}
	ch := make(chan result, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- result{err: fmt.Errorf("script panic: %v", r)}
			}
		}()

		if err := L.CallByParam(lua.P{
			Fn:      handleFn,
			NRet:    1,
			Protect: true,
		}, lua.LString(args)); err != nil {
			ch <- result{err: err}
			return
		}
		ret := L.Get(-1)
		L.Pop(1)
		if ret == lua.LNil {
			ch <- result{val: ""}
		} else {
			ch <- result{val: ret.String()}
		}
	}()

	select {
	case r := <-ch:
		stopMon()
		if memMon.wasExceeded() {
			return "", fmt.Errorf("script killed: memory limit exceeded")
		}
		return r.val, r.err
	case <-ctx.Done():
		stopMon()
		closeL()
		// Drain goroutine so it doesn't leak
		select {
		case <-ch:
		case <-time.After(500 * time.Millisecond):
		}
		if memMon.wasExceeded() {
			return "", fmt.Errorf("script killed: memory limit exceeded")
		}
		return "", fmt.Errorf("script timed out")
	}
}

// SetDB sets the database reference for goop.db access in data functions.
func (e *Engine) SetDB(db *storage.DB) {
	e.db = db
}

// registryMaxSize derives a registry cap from the MaxMemoryMB config.
// Each registry slot is roughly 48 bytes; this gives a proportional bound.
func (e *Engine) registryMaxSize() int {
	if e.cfg.MaxMemoryMB <= 0 {
		return 0
	}
	max := e.cfg.MaxMemoryMB * 1024 * 1024 / 48
	if max < 5120 {
		max = 5120
	}
	return max
}

// FunctionsDir returns the path to the data functions directory.
func (e *Engine) FunctionsDir() string {
	return e.functionsDir
}

// Commands returns a sorted list of loaded command names.
func (e *Engine) Commands() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cmds := make([]string, 0, len(e.scripts))
	for name := range e.scripts {
		cmds = append(cmds, name)
	}
	sort.Strings(cmds)
	return cmds
}

// ListDataFunctions returns metadata for scripts that define call().
func (e *Engine) ListDataFunctions() any {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var funcs []DataFunctionInfo
	for name, meta := range e.scripts {
		if meta.hasCall {
			funcs = append(funcs, DataFunctionInfo{
				Name:        name,
				Description: meta.description,
			})
		}
	}
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].Name < funcs[j].Name })
	return funcs
}

// CallFunction executes a script's call(request) entry point.
// This is the Phase 2 data function interface.
func (e *Engine) CallFunction(ctx context.Context, callerID, function string, params map[string]any) (any, error) {
	// Lookup script
	e.mu.RLock()
	meta, ok := e.scripts[function]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("function not found: %s", function)
	}
	if !meta.hasCall {
		return nil, fmt.Errorf("function %s does not support call()", function)
	}

	// Rate limit (per-function)
	if !e.limiter.AllowFunc(callerID, function, meta.rateLimit) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	// Resolve peer label
	peerLabel := callerID
	if callerID == e.selfID {
		peerLabel = e.selfLabel()
	} else if sp, ok := e.peers.Get(callerID); ok {
		peerLabel = sp.Content
	}

	// Execute with timeout
	timeout := time.Duration(e.cfg.TimeoutSeconds) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	inv := &invocationCtx{
		ctx:        execCtx,
		scriptName: function,
		peerID:     callerID,
		peerLabel:  peerLabel,
		selfID:     e.selfID,
		selfLabel:  e.selfLabel(),
	}

	return e.executeDataFunction(execCtx, inv, meta.proto, params)
}

func (e *Engine) executeDataFunction(ctx context.Context, inv *invocationCtx, proto *lua.FunctionProto, params map[string]any) (any, error) {
	L := newSandboxedDataVM(inv, e.kv, e, e.db)

	var closeOnce sync.Once
	closeL := func() { closeOnce.Do(func() { L.Close() }) }
	defer closeL()

	// Load compiled proto
	lfunc := L.NewFunctionFromProto(proto)
	L.Push(lfunc)
	if err := L.PCall(0, lua.MultRet, nil); err != nil {
		return nil, fmt.Errorf("load script: %w", err)
	}

	// Get call() function
	callFn := L.GetGlobal("call")
	if callFn == lua.LNil {
		return nil, fmt.Errorf("script has no call() function")
	}

	// Build request table
	requestTbl := L.NewTable()
	paramsTbl := goToLua(L, mapToInterface(params))
	requestTbl.RawSetString("params", paramsTbl)

	memMon := newMemoryMonitor(e.cfg.MaxMemoryMB)
	stopMon := memMon.watch(ctx, L, inv.scriptName)

	// Run in goroutine for timeout
	type result struct {
		val any
		err error
	}
	ch := make(chan result, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- result{err: fmt.Errorf("script panic: %v", r)}
			}
		}()

		if err := L.CallByParam(lua.P{
			Fn:      callFn,
			NRet:    1,
			Protect: true,
		}, requestTbl); err != nil {
			ch <- result{err: err}
			return
		}
		ret := L.Get(-1)
		L.Pop(1)
		ch <- result{val: luaToGo(ret)}
	}()

	select {
	case r := <-ch:
		stopMon()
		if memMon.wasExceeded() {
			return nil, fmt.Errorf("script killed: memory limit exceeded")
		}
		return r.val, r.err
	case <-ctx.Done():
		stopMon()
		closeL()
		select {
		case <-ch:
		case <-time.After(500 * time.Millisecond):
		}
		if memMon.wasExceeded() {
			return nil, fmt.Errorf("script killed: memory limit exceeded")
		}
		return nil, fmt.Errorf("script timed out")
	}
}

// mapToInterface converts map[string]any to interface{} for goToLua.
func mapToInterface(m map[string]any) interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Close shuts down the engine.
func (e *Engine) Close() {
	close(e.closed)
	e.watcher.Close()
	log.Printf("LUA: engine stopped")
}
