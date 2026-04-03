package lua

import (
	"github.com/petervdpas/goop2/internal/storage"

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

	// goop.group
	groupTbl := L.NewTable()
	groupTbl.RawSetString("is_member", L.NewFunction(groupIsMemberFn(inv, engine)))
	groupTbl.RawSetString("owner", L.NewFunction(groupOwnerFn(inv, engine)))
	memberTbl := L.NewTable()
	memberTbl.RawSetString("id", lua.LString(inv.peerID))
	memberTbl.RawSetString("role", L.NewFunction(groupMemberRoleFn(inv, engine)))
	groupTbl.RawSetString("member", memberTbl)
	groupTbl.RawSetString("create", L.NewFunction(groupCreateFn(engine)))
	groupTbl.RawSetString("close", L.NewFunction(groupCloseFn(engine)))
	groupTbl.RawSetString("add", L.NewFunction(groupAddFn(engine)))
	groupTbl.RawSetString("remove", L.NewFunction(groupRemoveFn(engine)))
	groupTbl.RawSetString("members", L.NewFunction(groupMembersFn(engine)))
	groupTbl.RawSetString("send", L.NewFunction(groupSendFn(engine)))
	groupTbl.RawSetString("list", L.NewFunction(groupListFn(engine)))
	goop.RawSetString("group", groupTbl)

	// goop.template
	templateTbl := L.NewTable()
	requireEmail := false
	if engine.db != nil {
		requireEmail = engine.db.GetMeta("template_require_email") == "1"
	}
	templateTbl.RawSetString("require_email", lua.LBool(requireEmail))
	goop.RawSetString("template", templateTbl)

	// goop.route / goop.owner / goop.expr
	goop.RawSetString("route", L.NewFunction(routeFn()))
	goop.RawSetString("owner", L.NewFunction(ownerFn(inv)))
	goop.RawSetString("expr", L.NewFunction(exprFn()))

	// goop.commands()
	goop.RawSetString("commands", L.NewFunction(commandsFn(engine)))

	// goop.listen
	listenTbl := L.NewTable()
	listenTbl.RawSetString("state", L.NewFunction(listenStateFn(engine)))
	listenTbl.RawSetString("create", L.NewFunction(listenCreateFn(engine)))
	listenTbl.RawSetString("close", L.NewFunction(listenCloseFn(engine)))
	listenTbl.RawSetString("load", L.NewFunction(listenLoadFn(engine)))
	listenTbl.RawSetString("play", L.NewFunction(listenPlayFn(engine)))
	listenTbl.RawSetString("pause", L.NewFunction(listenPauseFn(engine)))
	listenTbl.RawSetString("seek", L.NewFunction(listenSeekFn(engine)))
	goop.RawSetString("listen", listenTbl)

	L.SetGlobal("goop", goop)
}

// newSandboxedDataVM creates a VM with goop.db access for data functions.
func newSandboxedDataVM(inv *invocationCtx, kv *kvStore, engine *Engine, db *storage.DB) *lua.LState {
	L := newSandboxedVM(inv, kv, engine)

	// Inject goop.db and goop.schema tables (only for data functions)
	if db != nil {
		goop := L.GetGlobal("goop")
		goopTbl, ok := goop.(*lua.LTable)
		if ok {
			dbTbl := L.NewTable()
			dbTbl.RawSetString("query", L.NewFunction(dbQueryFn(inv, db)))
			dbTbl.RawSetString("scalar", L.NewFunction(dbScalarFn(inv, db)))
			dbTbl.RawSetString("exec", L.NewFunction(dbExecFn(inv, db)))
			goopTbl.RawSetString("db", dbTbl)

			goopTbl.RawSetString("orm", L.NewFunction(ormFn(inv, db)))
			goopTbl.RawSetString("config", L.NewFunction(configFn(inv, db)))

			if engine.content != nil {
				siteTbl := L.NewTable()
				siteTbl.RawSetString("read", L.NewFunction(siteReadFn(engine.content)))
				goopTbl.RawSetString("site", siteTbl)
			}

			if engine.peerDir != "" {
				txTbl := L.NewTable()
				txTbl.RawSetString("load", L.NewFunction(transformLoadFn(engine.peerDir)))
				txTbl.RawSetString("apply", L.NewFunction(transformApplyFn(engine.peerDir)))
				txTbl.RawSetString("apply_many", L.NewFunction(transformApplyManyFn(engine.peerDir)))
				txTbl.RawSetString("list", L.NewFunction(transformListFn(engine.peerDir)))
				txTbl.RawSetString("transforms", L.NewFunction(transformNamesFn))
				goopTbl.RawSetString("transform", txTbl)
			}
		}
	}

	return L
}
