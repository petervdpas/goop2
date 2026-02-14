package lua

import (
	"github.com/petervdpas/goop2/internal/listen"

	lua "github.com/yuin/gopher-lua"
)

// listenStateFn returns the current room state as a Lua table.
//
//	result, err = goop.listen.state()
func listenStateFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("listen not available"))
			return 2
		}
		room := lm.GetRoom()
		if room == nil {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 2
		}
		L.Push(roomToLua(L, room))
		L.Push(lua.LNil)
		return 2
	}
}

// listenCreateFn creates a new listening room.
//
//	result, err = goop.listen.create("My Room")
func listenCreateFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("listen not available"))
			return 2
		}
		name := L.OptString(1, "Listening Room")
		room, err := lm.CreateRoom(name)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(roomToLua(L, room))
		L.Push(lua.LNil)
		return 2
	}
}

// listenCloseFn closes the current room.
//
//	err = goop.listen.close()
func listenCloseFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LString("listen not available"))
			return 1
		}
		if err := lm.CloseRoom(); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// listenLoadFn loads an MP3 track.
//
//	track, err = goop.listen.load("/path/to/track.mp3")
func listenLoadFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("listen not available"))
			return 2
		}
		path := L.CheckString(1)
		track, err := lm.LoadTrack(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(trackToLua(L, track))
		L.Push(lua.LNil)
		return 2
	}
}

// listenPlayFn starts playback.
//
//	err = goop.listen.play()
func listenPlayFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LString("listen not available"))
			return 1
		}
		if err := lm.Play(); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// listenPauseFn pauses playback.
//
//	err = goop.listen.pause()
func listenPauseFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LString("listen not available"))
			return 1
		}
		if err := lm.Pause(); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// listenSeekFn seeks to a position in seconds.
//
//	err = goop.listen.seek(30.5)
func listenSeekFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		lm := engine.listen
		if lm == nil {
			L.Push(lua.LString("listen not available"))
			return 1
		}
		pos := float64(L.CheckNumber(1))
		if err := lm.Seek(pos); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// ── Lua table builders ───────────────────────────────────────────────────────

func roomToLua(L *lua.LState, r *listen.Room) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("id", lua.LString(r.ID))
	tbl.RawSetString("name", lua.LString(r.Name))
	tbl.RawSetString("role", lua.LString(r.Role))

	if r.Track != nil {
		tbl.RawSetString("track", trackToLua(L, r.Track))
	}

	if r.PlayState != nil {
		ps := L.NewTable()
		ps.RawSetString("playing", lua.LBool(r.PlayState.Playing))
		ps.RawSetString("position", lua.LNumber(r.PlayState.Position))
		ps.RawSetString("updated_at", lua.LNumber(r.PlayState.UpdatedAt))
		tbl.RawSetString("play_state", ps)
	}

	if len(r.Listeners) > 0 {
		listeners := L.NewTable()
		for i, pid := range r.Listeners {
			listeners.RawSetInt(i+1, lua.LString(pid))
		}
		tbl.RawSetString("listeners", listeners)
	}

	return tbl
}

func trackToLua(L *lua.LState, t *listen.Track) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString(t.Name))
	tbl.RawSetString("duration", lua.LNumber(t.Duration))
	tbl.RawSetString("bitrate", lua.LNumber(t.Bitrate))
	tbl.RawSetString("format", lua.LString(t.Format))
	return tbl
}
