package lua

import (
	"path/filepath"

	"github.com/petervdpas/goop2/internal/orm/mapper"
	"github.com/petervdpas/goop2/internal/orm/schema"

	lua "github.com/yuin/gopher-lua"
)

func mapperLoadFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		dir := filepath.Join(peerDir, "mappings")

		m, err := mapper.Load(dir, name)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := mappingToLua(L, m)
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

func mapperApplyFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		rowTbl := L.CheckTable(2)

		dir := filepath.Join(peerDir, "mappings")
		m, err := mapper.Load(dir, name)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		row := luaTableToSchemaRow(rowTbl)
		result, err := m.Apply(row)
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

func mapperApplyManyFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		name := L.CheckString(1)
		rowsTbl := L.CheckTable(2)

		dir := filepath.Join(peerDir, "mappings")
		m, err := mapper.Load(dir, name)
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

		results, err := m.ApplyMany(rows)
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

func mapperListFn(peerDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		dir := filepath.Join(peerDir, "mappings")
		mappings, err := mapper.LoadDir(dir)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		tbl := L.NewTable()
		for i, m := range mappings {
			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(m.Name))
			entry.RawSetString("description", lua.LString(m.Description))
			entry.RawSetString("field_count", lua.LNumber(float64(len(m.Fields))))
			tbl.RawSetInt(i+1, entry)
		}
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

func mapperTransformsFn(L *lua.LState) int {
	names := mapper.TransformNames()
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

func mappingToLua(L *lua.LState, m *mapper.Mapping) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString(m.Name))
	tbl.RawSetString("description", lua.LString(m.Description))

	fieldsTbl := L.NewTable()
	for i, f := range m.Fields {
		ft := L.NewTable()
		ft.RawSetString("target", lua.LString(f.Target))
		if len(f.Sources) > 0 {
			srcTbl := L.NewTable()
			for j, s := range f.Sources {
				srcTbl.RawSetInt(j+1, lua.LString(s))
			}
			ft.RawSetString("sources", srcTbl)
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
