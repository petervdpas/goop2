package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/goop2/internal/orm/schema"
	"github.com/petervdpas/goop2/internal/storage"

	lua "github.com/yuin/gopher-lua"
)

// invocationCtx holds per-invocation state shared by API functions.
type invocationCtx struct {
	ctx        context.Context
	scriptName string
	peerID     string
	peerLabel  string
	peerEmail  string
	selfID     string
	selfLabel  string

	httpCount int // requests made this invocation
}

const (
	maxHTTPPerInvocation = 3
	maxHTTPResponseBytes = 1 * 1024 * 1024 // 1MB
	maxKVKeys            = 1000
	maxKVBytes           = 64 * 1024 // 64KB
)

// kvStore manages per-script key-value state persisted as JSON files.
type kvStore struct {
	mu       sync.Mutex
	stateDir string
}

func newKVStore(stateDir string) *kvStore {
	return &kvStore{stateDir: stateDir}
}

func (kv *kvStore) path(scriptName string) string {
	return filepath.Join(kv.stateDir, scriptName+".json")
}

func (kv *kvStore) load(scriptName string) (map[string]interface{}, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	data, err := os.ReadFile(kv.path(scriptName))
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]interface{}), nil
	}
	return m, nil
}

func (kv *kvStore) save(scriptName string, m map[string]interface{}) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > maxKVBytes {
		return fmt.Errorf("kv store exceeds %dKB limit", maxKVBytes/1024)
	}
	return os.WriteFile(kv.path(scriptName), data, 0644)
}

// ── HTTP API ──

func httpGetFn(inv *invocationCtx) lua.LGFunction {
	return func(L *lua.LState) int {
		url := L.CheckString(1)
		body, err := doHTTPRequest(inv, "GET", url, "")
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(body))
		L.Push(lua.LNil)
		return 2
	}
}

func httpPostFn(inv *invocationCtx) lua.LGFunction {
	return func(L *lua.LState) int {
		url := L.CheckString(1)
		payload := L.OptString(2, "")
		body, err := doHTTPRequest(inv, "POST", url, payload)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(body))
		L.Push(lua.LNil)
		return 2
	}
}

func doHTTPRequest(inv *invocationCtx, method, rawURL, payload string) (string, error) {
	inv.httpCount++
	if inv.httpCount > maxHTTPPerInvocation {
		return "", fmt.Errorf("http request limit (%d) exceeded", maxHTTPPerInvocation)
	}

	var bodyReader io.Reader
	if payload != "" {
		bodyReader = strings.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(inv.ctx, method, rawURL, bodyReader)
	if err != nil {
		return "", err
	}
	if method == "POST" && payload != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// Use an SSRF-safe client that pins DNS resolution in the dialer,
	// eliminating the TOCTOU window of DNS rebinding attacks.
	client := &http.Client{
		Timeout:   HTTPTimeout,
		Transport: ssrfSafeTransport(),
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseBytes))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ssrfSafeTransport returns an http.Transport with a custom dialer that
// resolves DNS and validates the IP before connecting, preventing DNS
// rebinding attacks (TOCTOU between lookup and connect).
func ssrfSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}

			// Resolve DNS
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("dns lookup failed: %w", err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no addresses for host %s", host)
			}

			// Validate ALL resolved IPs before connecting to any
			for _, ipAddr := range ips {
				if err := checkIP(ipAddr.IP); err != nil {
					return nil, err
				}
			}

			// Connect directly to the validated IP, bypassing further DNS
			var dialer net.Dialer
			pinnedAddr := net.JoinHostPort(ips[0].IP.String(), port)
			return dialer.DialContext(ctx, network, pinnedAddr)
		},
	}
}

// checkIP rejects loopback, private, and link-local addresses.
func checkIP(ip net.IP) error {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("request to private/loopback address blocked")
	}
	return nil
}

// ── JSON API ──

func jsonDecodeFn(L *lua.LState) int {
	str := L.CheckString(1)
	var v interface{}
	if err := json.Unmarshal([]byte(str), &v); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(goToLua(L, v))
	L.Push(lua.LNil)
	return 2
}

func jsonEncodeFn(L *lua.LState) int {
	lv := L.CheckAny(1)
	v := luaToGo(lv)
	data, err := json.Marshal(v)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}

func goToLua(L *lua.LState, v interface{}) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case bool:
		return lua.LBool(val)
	case float64:
		return lua.LNumber(val)
	case int:
		return lua.LNumber(float64(val))
	case int64:
		return lua.LNumber(float64(val))
	case string:
		return lua.LString(val)
	case []interface{}:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, goToLua(L, item))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, goToLua(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", val))
	}
}

func luaToGo(lv lua.LValue) interface{} {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		// Check for SQL expression marker
		if expr := v.RawGetString("__sql_expr"); expr != lua.LNil {
			if s, ok := expr.(lua.LString); ok {
				return storage.SQLExpr{Expr: string(s)}
			}
		}
		// Check if it's an array (sequential integer keys starting at 1)
		maxN := v.MaxN()
		if maxN > 0 {
			arr := make([]interface{}, 0, maxN)
			for i := 1; i <= maxN; i++ {
				arr = append(arr, luaToGo(v.RawGetInt(i)))
			}
			return arr
		}
		// Check if the table has any keys at all
		hasKeys := false
		v.ForEach(func(_, _ lua.LValue) { hasKeys = true })
		if !hasKeys {
			return []interface{}{}
		}
		// Otherwise treat as map
		m := make(map[string]interface{})
		v.ForEach(func(key, val lua.LValue) {
			if ks, ok := key.(lua.LString); ok {
				m[string(ks)] = luaToGo(val)
			} else {
				m[fmt.Sprintf("%v", key)] = luaToGo(val)
			}
		})
		return m
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ── KV API ──

func kvGetFn(inv *invocationCtx, kv *kvStore) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		m, err := kv.load(inv.scriptName)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		val, ok := m[key]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLua(L, val))
		return 1
	}
}

func kvSetFn(inv *invocationCtx, kv *kvStore) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		val := luaToGo(L.CheckAny(2))

		m, err := kv.load(inv.scriptName)
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		if _, exists := m[key]; !exists && len(m) >= maxKVKeys {
			L.Push(lua.LString(fmt.Sprintf("kv store key limit (%d) exceeded", maxKVKeys)))
			return 1
		}
		m[key] = val
		if err := kv.save(inv.scriptName, m); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

func kvDelFn(inv *invocationCtx, kv *kvStore) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		m, err := kv.load(inv.scriptName)
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		delete(m, key)
		if err := kv.save(inv.scriptName, m); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// ── Log API ──

func logInfoFn(L *lua.LState) int {
	msg := L.CheckString(1)
	log.Printf("LUA [info] %s", msg)
	return 0
}

func logWarnFn(L *lua.LState) int {
	msg := L.CheckString(1)
	log.Printf("LUA [warn] %s", msg)
	return 0
}

func logErrorFn(L *lua.LState) int {
	msg := L.CheckString(1)
	log.Printf("LUA [error] %s", msg)
	return 0
}

// ── Commands API ──

func commandsFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		cmds := engine.Commands()
		tbl := L.NewTable()
		sort.Strings(cmds)
		for i, name := range cmds {
			tbl.RawSetInt(i+1, lua.LString(name))
		}
		L.Push(tbl)
		return 1
	}
}

// ── DB API (Phase 2 — data functions only) ──

func dbQueryFn(_ *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		query := L.CheckString(1)

		// Collect variadic args (query parameters)
		args := collectLuaArgs(L, 2)

		rows, err := db.LuaQuery(query, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Convert []map[string]any to Lua table of tables
		tbl := L.NewTable()
		for i, row := range rows {
			rowTbl := L.NewTable()
			for k, v := range row {
				rowTbl.RawSetString(k, goToLua(L, v))
			}
			tbl.RawSetInt(i+1, rowTbl)
		}

		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

func dbScalarFn(_ *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		query := L.CheckString(1)

		// Collect variadic args (query parameters)
		args := collectLuaArgs(L, 2)

		val, err := db.LuaScalar(query, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(goToLua(L, val))
		L.Push(lua.LNil)
		return 2
	}
}

func dbExecFn(_ *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		stmt := L.CheckString(1)

		// Collect variadic args (statement parameters)
		args := collectLuaArgs(L, 2)

		affected, err := db.LuaExec(stmt, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LNumber(affected))
		L.Push(lua.LNil)
		return 2
	}
}

// schemaToLua converts a schema.Table to a Lua table with full metadata.
func schemaToLua(L *lua.LState, tbl *schema.Table) *lua.LTable {
	result := L.NewTable()
	result.RawSetString("name", lua.LString(tbl.Name))
	result.RawSetString("system_key", lua.LBool(tbl.SystemKey))
	result.RawSetString("context", lua.LBool(tbl.Context))

	colsTbl := L.NewTable()
	for i, c := range tbl.Columns {
		colTbl := L.NewTable()
		colTbl.RawSetString("name", lua.LString(c.Name))
		colTbl.RawSetString("type", lua.LString(c.Type))
		colTbl.RawSetString("key", lua.LBool(c.Key))
		colTbl.RawSetString("required", lua.LBool(c.Required))
		colTbl.RawSetString("auto", lua.LBool(c.Auto))
		if c.Default != nil {
			colTbl.RawSetString("default", goToLua(L, c.Default))
		}
		if len(c.Values) > 0 {
			valsTbl := L.NewTable()
			for j, v := range c.Values {
				vTbl := L.NewTable()
				vTbl.RawSetString("key", lua.LString(v.Key))
				vTbl.RawSetString("label", lua.LString(v.Label))
				valsTbl.RawSetInt(j+1, vTbl)
			}
			colTbl.RawSetString("values", valsTbl)
		}
		colsTbl.RawSetInt(i+1, colTbl)
	}
	result.RawSetString("columns", colsTbl)

	if tbl.Access != nil {
		accessTbl := L.NewTable()
		accessTbl.RawSetString("read", lua.LString(tbl.Access.Read))
		accessTbl.RawSetString("insert", lua.LString(tbl.Access.Insert))
		accessTbl.RawSetString("update", lua.LString(tbl.Access.Update))
		accessTbl.RawSetString("delete", lua.LString(tbl.Access.Delete))
		result.RawSetString("access", accessTbl)
	}

	return result
}

// exprFn implements goop.expr(sql) — returns a marker table for raw SQL expressions.
// Used in update_where data to emit "position + 1" instead of binding as a string.
func exprFn() lua.LGFunction {
	return func(L *lua.LState) int {
		expr := L.CheckString(1)
		tbl := L.NewTable()
		tbl.RawSetString("__sql_expr", lua.LString(expr))
		L.Push(tbl)
		return 1
	}
}

// routeFn implements goop.route(actions) — action dispatcher.
// Takes a table of action_name → function mappings. Returns a function that
// extracts "action" from request.params and dispatches to the right handler.
// Usage: function call(req) return goop.route({ list = list_fn, save = save_fn })(req) end
// Or assign: local dispatch = goop.route({...}); function call(req) return dispatch(req) end
func routeFn() lua.LGFunction {
	return func(L *lua.LState) int {
		actionsTbl := L.CheckTable(1)

		dispatcher := L.NewFunction(func(L *lua.LState) int {
			req := L.CheckTable(1)

			paramsTbl := req.RawGetString("params")
			params, ok := paramsTbl.(*lua.LTable)
			if !ok {
				L.RaiseError("request.params missing")
				return 0
			}

			actionVal := params.RawGetString("action")
			action, ok := actionVal.(lua.LString)
			if !ok || string(action) == "" {
				L.RaiseError("action parameter required")
				return 0
			}

			handler := actionsTbl.RawGetString(string(action))
			if handler == lua.LNil {
				L.RaiseError("unknown action: %s", string(action))
				return 0
			}

			fn, ok := handler.(*lua.LFunction)
			if !ok {
				L.RaiseError("action %s is not a function", string(action))
				return 0
			}

			top := L.GetTop()
			L.Push(fn)
			L.Push(params)
			L.Call(1, lua.MultRet)
			return L.GetTop() - top
		})

		L.Push(dispatcher)
		return 1
	}
}

// ownerFn implements goop.owner(fn) — owner-only wrapper.
// Returns a new function that errors if goop.peer.id ~= goop.self.id,
// otherwise calls the wrapped function with the same arguments.
func ownerFn(inv *invocationCtx) lua.LGFunction {
	return func(L *lua.LState) int {
		fn := L.CheckFunction(1)

		wrapped := L.NewFunction(func(L *lua.LState) int {
			if inv.peerID != inv.selfID {
				L.RaiseError("only the site owner can do this")
				return 0
			}
			nArgs := L.GetTop()
			top := nArgs
			L.Push(fn)
			for i := 1; i <= nArgs; i++ {
				L.Push(L.Get(i))
			}
			L.Call(nArgs, lua.MultRet)
			return L.GetTop() - top
		})

		L.Push(wrapped)
		return 1
	}
}

// groupMemberRoleFn implements goop.group.member.role — returns the calling
// peer's role in the template group ("owner", "editor", "viewer", or "").
func groupMemberRoleFn(inv *invocationCtx, engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		role := peerGroupRole(inv, engine)
		L.Push(lua.LString(role))
		return 1
	}
}

// groupIsMemberFn implements goop.group.is_member() — returns true if the
// calling peer is a member of the template group (any role).
func groupIsMemberFn(inv *invocationCtx, engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if peerGroupRole(inv, engine) != "" {
			L.Push(lua.LTrue)
		} else {
			L.Push(lua.LFalse)
		}
		return 1
	}
}

// groupOwnerFn implements goop.group.owner — returns the group owner's peer ID.
func groupOwnerFn(inv *invocationCtx, engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groups != nil {
			L.Push(lua.LString(engine.groups.TemplateGroupOwner()))
		} else {
			L.Push(lua.LString(inv.selfID))
		}
		return 1
	}
}

// peerGroupRole returns the calling peer's role in the template group.
func peerGroupRole(inv *invocationCtx, engine *Engine) string {
	if inv.peerID == inv.selfID {
		return "owner"
	}
	if engine.groups != nil {
		return engine.groups.TemplateMemberRole(inv.peerID)
	}
	return ""
}


// groupCreateFn implements goop.group.create(name, type, context, max) → group_id
func groupCreateFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.RaiseError("group manager not available")
			return 0
		}
		name := L.CheckString(1)
		groupType := L.CheckString(2)
		groupContext := L.OptString(3, "")
		maxMembers := L.OptInt(4, 0)
		id := fmt.Sprintf("%x", time.Now().UnixNano())
		if err := engine.groupMgr.CreateGroup(id, name, groupType, groupContext, maxMembers, false); err != nil {
			L.RaiseError("create group: %s", err.Error())
			return 0
		}
		if err := engine.groupMgr.JoinOwnGroup(id); err != nil {
			log.Printf("LUA: group.create: auto-join failed: %v", err)
		}
		L.Push(lua.LString(id))
		return 1
	}
}

// groupCloseFn implements goop.group.close(group_id)
func groupCloseFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.RaiseError("group manager not available")
			return 0
		}
		groupID := L.CheckString(1)
		if err := engine.groupMgr.CloseGroup(groupID); err != nil {
			L.RaiseError("close group: %s", err.Error())
			return 0
		}
		L.Push(lua.LTrue)
		return 1
	}
}

// groupAddFn implements goop.group.add(group_id, peer_id)
func groupAddFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.RaiseError("group manager not available")
			return 0
		}
		groupID := L.CheckString(1)
		peerID := L.CheckString(2)
		if err := engine.groupMgr.InvitePeer(context.Background(), peerID, groupID); err != nil {
			L.RaiseError("add member: %s", err.Error())
			return 0
		}
		L.Push(lua.LTrue)
		return 1
	}
}

// groupRemoveFn implements goop.group.remove(group_id, peer_id)
func groupRemoveFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.RaiseError("group manager not available")
			return 0
		}
		groupID := L.CheckString(1)
		peerID := L.CheckString(2)
		if err := engine.groupMgr.KickMember(groupID, peerID); err != nil {
			L.RaiseError("remove member: %s", err.Error())
			return 0
		}
		L.Push(lua.LTrue)
		return 1
	}
}

// groupSetRoleFn implements goop.group.set_role(group_id, peer_id, role)
func groupSetRoleFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.RaiseError("group manager not available")
			return 0
		}
		groupID := L.CheckString(1)
		peerID := L.CheckString(2)
		role := L.CheckString(3)
		if err := engine.groupMgr.SetMemberRole(groupID, peerID, role); err != nil {
			L.RaiseError("set role: %s", err.Error())
			return 0
		}
		L.Push(lua.LTrue)
		return 1
	}
}

// groupTypesFn implements goop.group.types() → list of registered group type names
func groupTypesFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.Push(L.NewTable())
			return 1
		}
		types := engine.groupMgr.RegisteredTypes()
		tbl := L.NewTable()
		for i, t := range types {
			tbl.RawSetInt(i+1, lua.LString(t))
		}
		L.Push(tbl)
		return 1
	}
}

// groupMembersFn implements goop.group.members(group_id) → table of {peer_id, name, role}
func groupMembersFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.Push(L.NewTable())
			return 1
		}
		groupID := L.CheckString(1)
		members := engine.groupMgr.HostedGroupMembers(groupID)
		tbl := L.NewTable()
		for i, m := range members {
			entry := L.NewTable()
			entry.RawSetString("peer_id", lua.LString(m.PeerID))
			entry.RawSetString("role", lua.LString(m.Role))
			name := ""
			if m.PeerID == engine.selfID {
				name = engine.selfLabel()
			} else if sp, ok := engine.peers.Get(m.PeerID); ok && sp.Content != "" {
				name = sp.Content
			}
			if name != "" {
				entry.RawSetString("name", lua.LString(name))
			}
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		return 1
	}
}

// groupSendFn implements goop.group.send(group_id, payload)
func groupSendFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.RaiseError("group manager not available")
			return 0
		}
		groupID := L.CheckString(1)
		payload := luaToGo(L.Get(2))
		if err := engine.groupMgr.SendToGroupAsHost(groupID, payload); err != nil {
			L.RaiseError("send to group: %s", err.Error())
			return 0
		}
		L.Push(lua.LTrue)
		return 1
	}
}

// groupListFn implements goop.group.list() → table of hosted groups
func groupListFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		if engine.groupMgr == nil {
			L.Push(L.NewTable())
			return 1
		}
		rows, err := engine.groupMgr.ListHostedGroups()
		if err != nil {
			L.Push(L.NewTable())
			return 1
		}
		tbl := L.NewTable()
		for i, g := range rows {
			entry := L.NewTable()
			entry.RawSetString("id", lua.LString(g.ID))
			entry.RawSetString("name", lua.LString(g.Name))
			entry.RawSetString("group_type", lua.LString(g.GroupType))
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		return 1
	}
}

// ormFn implements goop.orm(table) — returns a schema-aware table handle.
// The handle carries schema metadata (columns, access, system_key) and
// scoped CRUD methods so callers never pass the table name again.
func ormFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)

		tbl, err := db.GetSchema(tableName)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if tbl == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("not an ORM table: " + tableName))
			return 2
		}

		handle := schemaToLua(L, tbl)

		handle.RawSetString("find", L.NewFunction(func(L *lua.LState) int {
			opts := L.OptTable(2, nil)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			if opts != nil {
				L.Push(opts)
			}
			return schemaFindFn(db)(L)
		}))

		handle.RawSetString("find_one", L.NewFunction(func(L *lua.LState) int {
			opts := L.OptTable(2, nil)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			if opts != nil {
				L.Push(opts)
			}
			return schemaFindOneFn(db)(L)
		}))

		handle.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
			id := L.CheckNumber(2)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(id)
			return schemaGetFn(db)(L)
		}))

		handle.RawSetString("get_by", L.NewFunction(func(L *lua.LState) int {
			col := L.CheckString(2)
			val := L.Get(3)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(lua.LString(col))
			L.Push(val)
			return schemaGetByFn(db)(L)
		}))

		handle.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
			limit := L.OptNumber(2, 0)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(limit)
			return schemaListFn(db)(L)
		}))

		handle.RawSetString("count", L.NewFunction(func(L *lua.LState) int {
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			return schemaCountFn(db)(L)
		}))

		handle.RawSetString("exists", L.NewFunction(func(L *lua.LState) int {
			opts := L.OptTable(2, nil)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			if opts != nil {
				L.Push(opts)
			}
			return schemaExistsFn(db)(L)
		}))

		handle.RawSetString("pluck", L.NewFunction(func(L *lua.LState) int {
			col := L.CheckString(2)
			opts := L.OptTable(3, nil)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(lua.LString(col))
			if opts != nil {
				L.Push(opts)
			}
			return schemaPluckFn(db)(L)
		}))

		handle.RawSetString("distinct", L.NewFunction(func(L *lua.LState) int {
			col := L.CheckString(2)
			opts := L.OptTable(3, nil)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(lua.LString(col))
			if opts != nil {
				L.Push(opts)
			}
			return schemaDistinctFn(db)(L)
		}))

		handle.RawSetString("aggregate", L.NewFunction(func(L *lua.LState) int {
			expr := L.CheckString(2)
			opts := L.OptTable(3, nil)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(lua.LString(expr))
			if opts != nil {
				L.Push(opts)
			}
			return schemaAggregateFn(db)(L)
		}))

		handle.RawSetString("insert", L.NewFunction(func(L *lua.LState) int {
			data := L.CheckTable(2)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(data)
			return schemaInsertFn(inv, db)(L)
		}))

		handle.RawSetString("update", L.NewFunction(func(L *lua.LState) int {
			id := L.CheckNumber(2)
			data := L.CheckTable(3)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(id)
			L.Push(data)
			return schemaUpdateFn(db)(L)
		}))

		handle.RawSetString("delete", L.NewFunction(func(L *lua.LState) int {
			id := L.CheckNumber(2)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(id)
			return schemaDeleteFn(db)(L)
		}))

		handle.RawSetString("update_where", L.NewFunction(func(L *lua.LState) int {
			data := L.CheckTable(2)
			opts := L.CheckTable(3)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(data)
			L.Push(opts)
			return schemaUpdateWhereFn(db)(L)
		}))

		handle.RawSetString("delete_where", L.NewFunction(func(L *lua.LState) int {
			opts := L.CheckTable(2)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(opts)
			return schemaDeleteWhereFn(db)(L)
		}))

		handle.RawSetString("upsert", L.NewFunction(func(L *lua.LState) int {
			keyCol := L.CheckString(2)
			data := L.CheckTable(3)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(lua.LString(keyCol))
			L.Push(data)
			return schemaUpsertFn(inv, db)(L)
		}))

		handle.RawSetString("seed", L.NewFunction(func(L *lua.LState) int {
			rows := L.CheckTable(2)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(rows)
			return schemaSeedFn(inv, db)(L)
		}))

		handle.RawSetString("validate", L.NewFunction(func(L *lua.LState) int {
			data := L.CheckTable(2)
			L.SetTop(0)
			L.Push(lua.LString(tableName))
			L.Push(data)
			return schemaValidateFn(db)(L)
		}))

		L.Push(handle)
		L.Push(lua.LNil)
		return 2
	}
}

// configFn implements goop.config(table, defaults) — config helper.
// Auto-detects key-value mode (table has "key"+"value" columns) vs single-row mode.
// Returns a table with __index for reads and a :set(k,v) method for writes.
func configFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		defaultsTbl := L.OptTable(2, L.NewTable())

		tbl, err := db.GetSchema(tableName)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if tbl == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("not an ORM table: " + tableName))
			return 2
		}

		isKV := false
		hasKey := false
		hasValue := false
		for _, c := range tbl.Columns {
			if c.Name == "key" {
				hasKey = true
			}
			if c.Name == "value" {
				hasValue = true
			}
		}
		if hasKey && hasValue {
			isKV = true
		}

		values := L.NewTable()

		defaultsTbl.ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				values.RawSetString(string(ks), v)
			}
		})

		if isKV {
			rows, qErr := db.Select(tableName, []string{"key", "value"}, "")
			if qErr == nil {
				for _, row := range rows {
					k, _ := row["key"].(string)
					v, _ := row["value"].(string)
					if k != "" {
						values.RawSetString(k, lua.LString(v))
					}
				}
			}
		} else {
			rows, qErr := db.SelectPaged(storage.SelectOpts{Table: tableName, Limit: 1, Order: "_id DESC"})
			if qErr == nil && len(rows) > 0 {
				for col, val := range rows[0] {
					if col == "_id" || col == "_owner" || col == "_owner_email" || col == "_created_at" || col == "_updated_at" {
						continue
					}
					values.RawSetString(col, goToLua(L, val))
				}
			}
		}

		handle := L.NewTable()

		data := L.NewTable()
		values.ForEach(func(k, v lua.LValue) {
			data.RawSet(k, v)
		})

		meta := L.NewTable()
		meta.RawSetString("__index", data)
		L.SetMetatable(handle, meta)

		if isKV {
			handle.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
				key := L.CheckString(2)
				val := L.Get(3)
				valStr := ""
				if val != lua.LNil {
					valStr = val.String()
				}
				_, uErr := db.Upsert(tableName, "key", inv.peerID, "", map[string]any{"key": key, "value": valStr})
				if uErr != nil {
					L.Push(lua.LNil)
					L.Push(lua.LString(uErr.Error()))
					return 2
				}
				data.RawSetString(key, lua.LString(valStr))
				L.Push(lua.LTrue)
				return 1
			}))
		} else {
			getRowID := func() int64 {
				rows, err := db.SelectPaged(storage.SelectOpts{Table: tableName, Columns: []string{"_id"}, Limit: 1, Order: "_id DESC"})
				if err != nil || len(rows) == 0 {
					return 0
				}
				id, _ := rows[0]["_id"].(int64)
				return id
			}

			handle.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
				key := L.CheckString(2)
				val := L.Get(3)
				goVal := luaToGo(val)

				id := getRowID()
				if id > 0 {
					db.UpdateRow(tableName, id, map[string]any{key: goVal})
				} else {
					db.Insert(tableName, inv.peerID, "", map[string]any{key: goVal})
				}

				data.RawSetString(key, val)
				L.Push(lua.LTrue)
				return 1
			}))

			handle.RawSetString("save", L.NewFunction(func(L *lua.LState) int {
				updateTbl := L.CheckTable(2)
				updateData := make(map[string]any)
				updateTbl.ForEach(func(k, v lua.LValue) {
					if ks, ok := k.(lua.LString); ok {
						updateData[string(ks)] = luaToGo(v)
						data.RawSetString(string(ks), v)
					}
				})

				id := getRowID()
				if id > 0 {
					db.UpdateRow(tableName, id, updateData)
				} else {
					db.Insert(tableName, inv.peerID, "", updateData)
				}

				L.Push(lua.LTrue)
				return 1
			}))
		}

		L.Push(handle)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaValidateFn validates data against ORM schema types.
func schemaValidateFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		dataTbl := L.CheckTable(2)

		data := make(map[string]any)
		dataTbl.ForEach(func(key, val lua.LValue) {
			if ks, ok := key.(lua.LString); ok {
				data[string(ks)] = luaToGo(val)
			}
		})

		if err := db.ValidateInsert(tableName, data); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaInsertFn inserts a row with ORM validation and auto-generated columns.
func schemaInsertFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		dataTbl := L.CheckTable(2)

		data := luaTableToMap(dataTbl)

		id, err := db.OrmInsert(tableName, inv.peerID, "", data)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LNumber(id))
		L.Push(lua.LNil)
		return 2
	}
}

// schemaGetFn implements goop.schema.get(table, id) — get by _id.
func schemaGetFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		id := L.CheckInt64(2)

		row, err := db.OrmGet(tableName, id)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for k, v := range row {
			tbl.RawSetString(k, goToLua(L, v))
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaListFn implements goop.schema.list(table, limit?) — list all rows.
func schemaListFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		limit := L.OptInt(2, 0)

		rows, err := db.OrmList(tableName, limit)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, row := range rows {
			rowTbl := L.NewTable()
			for k, v := range row {
				rowTbl.RawSetString(k, goToLua(L, v))
			}
			tbl.RawSetInt(i+1, rowTbl)
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaUpdateFn implements goop.schema.update(table, id, data) — typed update by _id.
func schemaUpdateFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		id := L.CheckInt64(2)
		dataTbl := L.CheckTable(3)

		data := luaTableToMap(dataTbl)

		if err := db.OrmUpdate(tableName, id, data); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaDeleteFn implements goop.schema.delete(table, id) — delete by _id.
func schemaDeleteFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		id := L.CheckInt64(2)

		if err := db.OrmDelete(tableName, id); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaFindFn implements goop.schema.find(table, opts) — filtered query with ordering and pagination.
// opts: {where, args, fields, order, limit, offset}
func schemaFindFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		optsTbl := L.OptTable(2, nil)

		opts := storage.SelectOpts{Table: tableName}
		if optsTbl != nil {
			if v := optsTbl.RawGetString("where"); v != lua.LNil {
				opts.Where = v.String()
			}
			if v := optsTbl.RawGetString("order"); v != lua.LNil {
				opts.Order = v.String()
			}
			if v := optsTbl.RawGetString("limit"); v != lua.LNil {
				if n, ok := v.(lua.LNumber); ok {
					opts.Limit = int(n)
				}
			}
			if v := optsTbl.RawGetString("offset"); v != lua.LNil {
				if n, ok := v.(lua.LNumber); ok {
					opts.Offset = int(n)
				}
			}
			if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					opts.Args = append(opts.Args, luaToGo(val))
				})
			}
			if v, ok := optsTbl.RawGetString("fields").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					if s, ok := val.(lua.LString); ok {
						opts.Columns = append(opts.Columns, string(s))
					}
				})
			}
		}

		rows, err := db.SelectPaged(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, row := range rows {
			rowTbl := L.NewTable()
			for k, v := range row {
				rowTbl.RawSetString(k, goToLua(L, v))
			}
			tbl.RawSetInt(i+1, rowTbl)
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaFindOneFn implements goop.schema.find_one(table, opts) — single row query.
// Same opts as find, but auto-sets limit=1 and returns the row directly (not an array).
func schemaFindOneFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		optsTbl := L.OptTable(2, nil)

		opts := storage.SelectOpts{Table: tableName, Limit: 1}
		if optsTbl != nil {
			if v := optsTbl.RawGetString("where"); v != lua.LNil {
				opts.Where = v.String()
			}
			if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					opts.Args = append(opts.Args, luaToGo(val))
				})
			}
			if v, ok := optsTbl.RawGetString("fields").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					if s, ok := val.(lua.LString); ok {
						opts.Columns = append(opts.Columns, string(s))
					}
				})
			}
		}

		rows, err := db.SelectPaged(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		if len(rows) == 0 {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 2
		}

		rowTbl := L.NewTable()
		for k, v := range rows[0] {
			rowTbl.RawSetString(k, goToLua(L, v))
		}
		L.Push(rowTbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaCountFn implements goop.schema.count(table) — returns row count.
func schemaCountFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		var n int64
		if err := db.QueryRow("SELECT COUNT(*) FROM "+tableName).Scan(&n); err != nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(n))
		L.Push(lua.LNil)
		return 2
	}
}

// schemaSeedFn implements goop.schema.seed(table, rows) — inserts rows only if the table is empty.
// rows is a Lua array of {col=val, ...} tables. Returns the number of rows inserted.
func schemaSeedFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		rowsTbl := L.CheckTable(2)

		var n int64
		if err := db.QueryRow("SELECT COUNT(*) FROM "+tableName).Scan(&n); err != nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if n > 0 {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNil)
			return 2
		}

		var inserted int
		rowsTbl.ForEach(func(_, val lua.LValue) {
			rowTbl, ok := val.(*lua.LTable)
			if !ok {
				return
			}
			data := luaTableToMap(rowTbl)
			if _, err := db.OrmInsert(tableName, inv.peerID, "", data); err != nil {
				return
			}
			inserted++
		})

		L.Push(lua.LNumber(inserted))
		L.Push(lua.LNil)
		return 2
	}
}

// schemaGetByFn implements goop.schema.get_by(table, column, value) — get single row by any column.
func schemaGetByFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		column := L.CheckString(2)
		value := luaToGo(L.Get(3))

		rows, err := db.SelectPaged(storage.SelectOpts{
			Table: tableName,
			Where: column + " = ?",
			Args:  []any{value},
			Limit: 1,
		})
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if len(rows) == 0 {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 2
		}

		tbl := L.NewTable()
		for k, v := range rows[0] {
			tbl.RawSetString(k, goToLua(L, v))
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaExistsFn implements goop.schema.exists(table, opts?) — check if any row matches.
func schemaExistsFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		optsTbl := L.OptTable(2, nil)

		var where string
		var args []any
		if optsTbl != nil {
			if v := optsTbl.RawGetString("where"); v != lua.LNil {
				where = v.String()
			}
			if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					args = append(args, luaToGo(val))
				})
			}
		}

		query := fmt.Sprintf("SELECT 1 FROM %s", tableName)
		if where != "" {
			query += " WHERE " + where
		}
		query += " LIMIT 1"

		var dummy int
		err := db.QueryRow(query, args...).Scan(&dummy)
		if err != nil {
			L.Push(lua.LFalse)
		} else {
			L.Push(lua.LTrue)
		}
		return 1
	}
}

// schemaPluckFn implements goop.schema.pluck(table, column, opts?) — returns flat array of one column's values.
func schemaPluckFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		column := L.CheckString(2)
		optsTbl := L.OptTable(3, nil)

		opts := storage.SelectOpts{
			Table:   tableName,
			Columns: []string{column},
		}
		if optsTbl != nil {
			if v := optsTbl.RawGetString("where"); v != lua.LNil {
				opts.Where = v.String()
			}
			if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					opts.Args = append(opts.Args, luaToGo(val))
				})
			}
			if v := optsTbl.RawGetString("order"); v != lua.LNil {
				opts.Order = v.String()
			}
			if v, ok := optsTbl.RawGetString("limit").(lua.LNumber); ok {
				opts.Limit = int(v)
			}
		}

		rows, err := db.SelectPaged(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, row := range rows {
			tbl.RawSetInt(i+1, goToLua(L, row[column]))
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaAggregateFn implements goop.schema.aggregate(table, expr, opts?) — aggregate queries.
// expr: "COUNT(*)" or "SUM(score), COUNT(*)" etc.
// opts: {where, args, group_by}
func schemaAggregateFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		expr := L.CheckString(2)
		optsTbl := L.OptTable(3, nil)

		var where, groupBy string
		var args []any
		if optsTbl != nil {
			if v := optsTbl.RawGetString("where"); v != lua.LNil {
				where = v.String()
			}
			if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					args = append(args, luaToGo(val))
				})
			}
			if v := optsTbl.RawGetString("group_by"); v != lua.LNil {
				groupBy = v.String()
			}
		}

		var rows []map[string]any
		var err error
		if groupBy != "" {
			rows, err = db.AggregateGroupBy(tableName, expr, groupBy, where, args...)
		} else {
			rows, err = db.Aggregate(tableName, expr, where, args...)
		}
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, row := range rows {
			rowTbl := L.NewTable()
			for k, v := range row {
				rowTbl.RawSetString(k, goToLua(L, v))
			}
			tbl.RawSetInt(i+1, rowTbl)
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaDistinctFn implements goop.schema.distinct(table, column, opts?) — unique values.
func schemaDistinctFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		column := L.CheckString(2)
		optsTbl := L.OptTable(3, nil)

		var where string
		var args []any
		if optsTbl != nil {
			if v := optsTbl.RawGetString("where"); v != lua.LNil {
				where = v.String()
			}
			if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
				v.ForEach(func(_, val lua.LValue) {
					args = append(args, luaToGo(val))
				})
			}
		}

		vals, err := db.Distinct(tableName, column, where, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, v := range vals {
			tbl.RawSetInt(i+1, goToLua(L, v))
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaUpdateWhereFn implements goop.schema.update_where(table, data, opts) — bulk update.
// opts: {where, args}
func schemaUpdateWhereFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		dataTbl := L.CheckTable(2)
		optsTbl := L.CheckTable(3)

		data := luaTableToMap(dataTbl)
		where := ""
		var args []any
		if v := optsTbl.RawGetString("where"); v != lua.LNil {
			where = v.String()
		}
		if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
			v.ForEach(func(_, val lua.LValue) {
				args = append(args, luaToGo(val))
			})
		}
		if where == "" {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString("update_where requires a where clause"))
			return 2
		}

		n, err := db.UpdateWhere(tableName, data, where, args...)
		if err != nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(n))
		L.Push(lua.LNil)
		return 2
	}
}

// schemaDeleteWhereFn implements goop.schema.delete_where(table, opts) — bulk delete.
// opts: {where, args}
func schemaDeleteWhereFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		optsTbl := L.CheckTable(2)

		where := ""
		var args []any
		if v := optsTbl.RawGetString("where"); v != lua.LNil {
			where = v.String()
		}
		if v, ok := optsTbl.RawGetString("args").(*lua.LTable); ok {
			v.ForEach(func(_, val lua.LValue) {
				args = append(args, luaToGo(val))
			})
		}
		if where == "" {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString("delete_where requires a where clause"))
			return 2
		}

		n, err := db.DeleteWhere(tableName, where, args...)
		if err != nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(n))
		L.Push(lua.LNil)
		return 2
	}
}

// schemaUpsertFn implements goop.schema.upsert(table, key_col, data) — insert or update on conflict.
func schemaUpsertFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		keyCol := L.CheckString(2)
		dataTbl := L.CheckTable(3)

		data := luaTableToMap(dataTbl)
		id, err := db.Upsert(tableName, keyCol, inv.peerID, "", data)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(id))
		L.Push(lua.LNil)
		return 2
	}
}

func luaTableToMap(tbl *lua.LTable) map[string]any {
	data := make(map[string]any)
	tbl.ForEach(func(key, val lua.LValue) {
		if ks, ok := key.(lua.LString); ok {
			data[string(ks)] = luaToGo(val)
		}
	})
	return data
}


// collectLuaArgs gathers variadic arguments from the Lua stack starting at position start.
func collectLuaArgs(L *lua.LState, start int) []any {
	var args []any
	for i := start; i <= L.GetTop(); i++ {
		args = append(args, luaToGo(L.Get(i)))
	}
	return args
}
