package lua

import (
	"path/filepath"

	"github.com/petervdpas/goop2/internal/orm/transformation"
	"github.com/petervdpas/goop2/internal/orm/schema"

	lua "github.com/yuin/gopher-lua"
)

func transformLoadFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		dir := filepath.Join(peerDir, "transformations")

		t, err := transformation.Load(dir, name)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := transformationToLua(L, t)
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

func transformApplyFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		rowTbl := L.CheckTable(2)

		dir := filepath.Join(peerDir, "transformations")
		t, err := transformation.Load(dir, name)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		row := luaTableToSchemaRow(rowTbl)
		result, err := t.Apply(row)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(schemaRowToLua(L, result))
		L.Push(lua.LNil)
		return 2
	}
}

func transformApplyManyFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		rowsTbl := L.CheckTable(2)

		dir := filepath.Join(peerDir, "transformations")
		t, err := transformation.Load(dir, name)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		var rows []schema.Row
		rowsTbl.ForEach(func(_, val lua.LValue) {
			if tbl, ok := val.(*lua.LTable); ok {
				rows = append(rows, luaTableToSchemaRow(tbl))
			}
		})

		results, err := t.ApplyMany(rows)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		outTbl := L.NewTable()
		for i, r := range results {
			outTbl.RawSetInt(i+1, schemaRowToLua(L, r))
		}
		L.Push(outTbl)
		L.Push(lua.LNil)
		return 2
	}
}

func transformListFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		dir := filepath.Join(peerDir, "transformations")
		items, err := transformation.LoadDir(dir)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, t := range items {
			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(t.Name))
			entry.RawSetString("description", lua.LString(t.Description))
			entry.RawSetString("field_count", lua.LNumber(float64(len(t.Fields))))
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

func transformNamesFn(L *lua.LState) int {
	names := transformation.TransformNames()
	tbl := L.NewTable()
	for i, n := range names {
		tbl.RawSetInt(i+1, lua.LString(n))
	}
	L.Push(tbl)
	return 1
}

func luaTableToSchemaRow(tbl *lua.LTable) schema.Row {
	row := make(schema.Row)
	tbl.ForEach(func(key, val lua.LValue) {
		if ks, ok := key.(lua.LString); ok {
			row[string(ks)] = luaToGo(val)
		}
	})
	return row
}

func schemaRowToLua(L *lua.LState, row schema.Row) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range row {
		tbl.RawSetString(k, goToLua(L, v))
	}
	return tbl
}

func transformationToLua(L *lua.LState, t *transformation.Transformation) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString(t.Name))
	tbl.RawSetString("description", lua.LString(t.Description))

	srcTbl := L.NewTable()
	srcTbl.RawSetString("type", lua.LString(t.Source.Type))
	srcTbl.RawSetString("name", lua.LString(t.Source.Name))
	srcTbl.RawSetString("path", lua.LString(t.Source.Path))
	srcTbl.RawSetString("url", lua.LString(t.Source.URL))
	tbl.RawSetString("source", srcTbl)

	tgtTbl := L.NewTable()
	tgtTbl.RawSetString("type", lua.LString(t.Target.Type))
	tgtTbl.RawSetString("name", lua.LString(t.Target.Name))
	tgtTbl.RawSetString("path", lua.LString(t.Target.Path))
	tgtTbl.RawSetString("url", lua.LString(t.Target.URL))
	tbl.RawSetString("target", tgtTbl)

	fieldsTbl := L.NewTable()
	for i, f := range t.Fields {
		ft := L.NewTable()
		ft.RawSetString("target", lua.LString(f.Target))
		if len(f.Sources) > 0 {
			sTbl := L.NewTable()
			for j, s := range f.Sources {
				sTbl.RawSetInt(j+1, lua.LString(s))
			}
			ft.RawSetString("sources", sTbl)
		}
		if f.Transform != "" {
			ft.RawSetString("transform", lua.LString(f.Transform))
		}
		if f.Constant != nil {
			ft.RawSetString("constant", goToLua(L, f.Constant))
		}
		if len(f.Args) > 0 {
			argsTbl := L.NewTable()
			for j, a := range f.Args {
				argsTbl.RawSetInt(j+1, goToLua(L, a))
			}
			ft.RawSetString("args", argsTbl)
		}
		fieldsTbl.RawSetInt(i+1, ft)
	}
	tbl.RawSetString("fields", fieldsTbl)
	return tbl
}
