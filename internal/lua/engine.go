package lua

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"goop/internal/config"
	"goop/internal/state"

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

// Engine manages Lua scripts, hot reload, and command dispatch.
type Engine struct {
	mu        sync.RWMutex
	scripts   map[string]*lua.FunctionProto // command name -> compiled proto
	cfg       config.Lua
	scriptDir string
	kv        *kvStore
	watcher   *fsnotify.Watcher
	limiter   *rateLimiter
	selfID    string
	selfLabel func() string
	peers     *state.PeerTable
	closed    chan struct{}
}

// NewEngine creates and starts a Lua scripting engine.
func NewEngine(cfg config.Lua, peerDir string, selfID string, selfLabel func() string, peers *state.PeerTable) (*Engine, error) {
	scriptDir := filepath.Join(peerDir, cfg.ScriptDir)
	stateDir := filepath.Join(scriptDir, ".state")

	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return nil, fmt.Errorf("create script dir: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	e := &Engine{
		scripts:   make(map[string]*lua.FunctionProto),
		cfg:       cfg,
		scriptDir: scriptDir,
		kv:        newKVStore(stateDir),
		watcher:   watcher,
		limiter:   newRateLimiter(cfg.RateLimitPerPeer, cfg.RateLimitGlobal),
		selfID:    selfID,
		selfLabel: selfLabel,
		peers:     peers,
		closed:    make(chan struct{}),
	}

	// Initial scan
	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("read script dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".lua")
		if err := e.compileScript(name, filepath.Join(scriptDir, entry.Name())); err != nil {
			log.Printf("LUA: failed to compile %s: %v", entry.Name(), err)
		}
	}

	if err := watcher.Add(scriptDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch script dir: %w", err)
	}

	go e.watchLoop()

	log.Printf("LUA: engine started, %d script(s) loaded from %s", len(e.scripts), scriptDir)
	return e, nil
}

func (e *Engine) compileScript(name, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	chunk, err := parse.Parse(strings.NewReader(string(data)), name)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	e.mu.Lock()
	e.scripts[name] = proto
	e.mu.Unlock()

	log.Printf("LUA: compiled script %q", name)
	return nil
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

	// Rate limit
	if !e.limiter.Allow(fromPeerID) {
		_ = sender.SendDirect(ctx, fromPeerID, "Rate limit exceeded. Try again later.")
		return
	}

	// Lookup script
	e.mu.RLock()
	proto, ok := e.scripts[cmdName]
	e.mu.RUnlock()

	if !ok {
		reply := fmt.Sprintf("Unknown command: %s", cmdName)
		_ = sender.SendDirect(ctx, fromPeerID, reply)
		return
	}

	// Resolve peer label
	peerLabel := fromPeerID
	if sp, ok := e.peers.Get(fromPeerID); ok {
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

	result, err := e.executeScript(execCtx, inv, proto, args)
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
	defer L.Close()

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

	// Run in goroutine so we can kill on timeout
	type result struct {
		val string
		err error
	}
	ch := make(chan result, 1)

	go func() {
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
		return r.val, r.err
	case <-ctx.Done():
		L.Close() // kill the VM
		return "", fmt.Errorf("script timed out")
	}
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

// Close shuts down the engine.
func (e *Engine) Close() {
	close(e.closed)
	e.watcher.Close()
	log.Printf("LUA: engine stopped")
}
