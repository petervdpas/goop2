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

	"github.com/petervdpas/goop2/internal/storage"

	lua "github.com/yuin/gopher-lua"
)

// invocationCtx holds per-invocation state shared by API functions.
type invocationCtx struct {
	ctx        context.Context
	scriptName string
	peerID     string
	peerLabel  string
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
		// Check if it's an array (sequential integer keys starting at 1)
		maxN := v.MaxN()
		if maxN > 0 {
			arr := make([]interface{}, 0, maxN)
			for i := 1; i <= maxN; i++ {
				arr = append(arr, luaToGo(v.RawGetInt(i)))
			}
			return arr
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

func dbQueryFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
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

func dbScalarFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
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

func dbExecFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
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

// schemaCreateFn implements goop.schema.create(name, columns) — creates an ORM-managed table.
// columns is a Lua array of {name, type, key?, required?, default?} tables.
func schemaCreateFn(inv *invocationCtx, db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		colsTbl := L.CheckTable(2)

		var columns []storage.LuaSchemaColumn
		colsTbl.ForEach(func(_, val lua.LValue) {
			row, ok := val.(*lua.LTable)
			if !ok {
				return
			}
			col := storage.LuaSchemaColumn{
				Name: luaString(row, "name"),
				Type: luaString(row, "type"),
			}
			if v := row.RawGetString("key"); v == lua.LTrue {
				col.Key = true
			}
			if v := row.RawGetString("required"); v == lua.LTrue {
				col.Required = true
			}
			if v := row.RawGetString("default"); v != lua.LNil {
				col.Default = luaToGo(v)
			}
			columns = append(columns, col)
		})

		if err := db.CreateTableORMFromLua(name, columns); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LTrue)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaDescribeFn implements goop.schema.describe(table) — returns typed schema.
func schemaDescribeFn(db *storage.DB) lua.LGFunction {
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
			L.Push(lua.LString("not an ORM table"))
			return 2
		}

		result := L.NewTable()
		result.RawSetString("name", lua.LString(tbl.Name))
		colsTbl := L.NewTable()
		for i, c := range tbl.Columns {
			colTbl := L.NewTable()
			colTbl.RawSetString("name", lua.LString(c.Name))
			colTbl.RawSetString("type", lua.LString(c.Type))
			colTbl.RawSetString("key", lua.LBool(c.Key))
			colTbl.RawSetString("required", lua.LBool(c.Required))
			if c.Default != nil {
				colTbl.RawSetString("default", goToLua(L, c.Default))
			}
			colsTbl.RawSetInt(i+1, colTbl)
		}
		result.RawSetString("columns", colsTbl)

		L.Push(result)
		L.Push(lua.LNil)
		return 2
	}
}

// schemaValidateFn implements goop.schema.validate(table, data) — validates types.
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

// schemaIsORMFn implements goop.schema.is_orm(table) — checks if table is ORM-managed.
func schemaIsORMFn(db *storage.DB) lua.LGFunction {
	return func(L *lua.LState) int {
		tableName := L.CheckString(1)
		L.Push(lua.LBool(db.IsORM(tableName)))
		return 1
	}
}


// schemaInsertFn implements goop.schema.insert(table, data) — typed insert.
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

func luaTableToMap(tbl *lua.LTable) map[string]any {
	data := make(map[string]any)
	tbl.ForEach(func(key, val lua.LValue) {
		if ks, ok := key.(lua.LString); ok {
			data[string(ks)] = luaToGo(val)
		}
	})
	return data
}

func luaString(tbl *lua.LTable, key string) string {
	v := tbl.RawGetString(key)
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	return ""
}

// collectLuaArgs gathers variadic arguments from the Lua stack starting at position start.
func collectLuaArgs(L *lua.LState, start int) []any {
	var args []any
	for i := start; i <= L.GetTop(); i++ {
		args = append(args, luaToGo(L.Get(i)))
	}
	return args
}
