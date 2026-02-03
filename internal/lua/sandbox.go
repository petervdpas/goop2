package lua

import (
	"goop/internal/storage"

	lua "github.com/yuin/gopher-lua"
)

// newSandboxedVM creates a gopher-lua VM with restricted standard libraries
// and the goop.* API table injected.
func newSandboxedVM(inv *invocationCtx, kv *kvStore, engine *Engine) *lua.LState {
	L := lua.NewState(lua.Options{
		SkipOpenLibs:        true,
		CallStackSize:       128,
		RegistrySize:        2048,
		RegistryMaxSize:     engine.registryMaxSize(),
		RegistryGrowStep:    32,
		MinimizeStackMemory: true,
	})

	// Selectively open safe standard libraries
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
		{lua.OsLibName, lua.OpenOs},
	} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}

	// Prune os to only time/date/clock
	pruneOS(L)

	// Remove dangerous globals
	for _, name := range []string{"dofile", "loadfile", "require"} {
		L.SetGlobal(name, lua.LNil)
	}

	// Inject goop.* table
	injectGoopTable(L, inv, kv, engine)

	return L
}

// pruneOS removes all os functions except time, date, and clock.
func pruneOS(L *lua.LState) {
	os := L.GetGlobal("os")
	osTbl, ok := os.(*lua.LTable)
	if !ok {
		return
	}

	// Collect keys to keep
	keep := map[string]bool{"time": true, "date": true, "clock": true}
	var toRemove []string

	osTbl.ForEach(func(key, _ lua.LValue) {
		if ks, ok := key.(lua.LString); ok {
			if !keep[string(ks)] {
				toRemove = append(toRemove, string(ks))
			}
		}
	})

	for _, k := range toRemove {
		osTbl.RawSetString(k, lua.LNil)
	}
}

// injectGoopTable builds and injects the full goop.* API table.
func injectGoopTable(L *lua.LState, inv *invocationCtx, kv *kvStore, engine *Engine) {
	goop := L.NewTable()

	// goop.peer (info about the message sender)
	peerTbl := L.NewTable()
	peerTbl.RawSetString("id", lua.LString(inv.peerID))
	peerTbl.RawSetString("label", lua.LString(inv.peerLabel))
	goop.RawSetString("peer", peerTbl)

	// goop.self (info about this node)
	selfTbl := L.NewTable()
	selfTbl.RawSetString("id", lua.LString(inv.selfID))
	selfTbl.RawSetString("label", lua.LString(inv.selfLabel))
	goop.RawSetString("self", selfTbl)

	// goop.http
	httpTbl := L.NewTable()
	httpTbl.RawSetString("get", L.NewFunction(httpGetFn(inv)))
	httpTbl.RawSetString("post", L.NewFunction(httpPostFn(inv)))
	goop.RawSetString("http", httpTbl)

	// goop.json
	jsonTbl := L.NewTable()
	jsonTbl.RawSetString("decode", L.NewFunction(jsonDecodeFn))
	jsonTbl.RawSetString("encode", L.NewFunction(jsonEncodeFn))
	goop.RawSetString("json", jsonTbl)

	// goop.kv
	kvTbl := L.NewTable()
	kvTbl.RawSetString("get", L.NewFunction(kvGetFn(inv, kv)))
	kvTbl.RawSetString("set", L.NewFunction(kvSetFn(inv, kv)))
	kvTbl.RawSetString("del", L.NewFunction(kvDelFn(inv, kv)))
	goop.RawSetString("kv", kvTbl)

	// goop.log
	logTbl := L.NewTable()
	logTbl.RawSetString("info", L.NewFunction(logInfoFn))
	logTbl.RawSetString("warn", L.NewFunction(logWarnFn))
	logTbl.RawSetString("error", L.NewFunction(logErrorFn))
	goop.RawSetString("log", logTbl)

	// goop.commands()
	goop.RawSetString("commands", L.NewFunction(commandsFn(engine)))

	L.SetGlobal("goop", goop)
}

// newSandboxedDataVM creates a VM with goop.db access for data functions.
func newSandboxedDataVM(inv *invocationCtx, kv *kvStore, engine *Engine, db *storage.DB) *lua.LState {
	L := newSandboxedVM(inv, kv, engine)

	// Inject goop.db table (only for data functions)
	if db != nil {
		goop := L.GetGlobal("goop")
		goopTbl, ok := goop.(*lua.LTable)
		if ok {
			dbTbl := L.NewTable()
			dbTbl.RawSetString("query", L.NewFunction(dbQueryFn(inv, db)))
			dbTbl.RawSetString("scalar", L.NewFunction(dbScalarFn(inv, db)))
			dbTbl.RawSetString("exec", L.NewFunction(dbExecFn(inv, db)))
			goopTbl.RawSetString("db", dbTbl)
		}
	}

	return L
}
