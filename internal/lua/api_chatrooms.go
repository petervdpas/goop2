package lua

import (
	chatType "github.com/petervdpas/goop2/internal/group_types/chat"

	lua "github.com/yuin/gopher-lua"
)

// chatRoomCreateFn creates a new chat room.
//
//	room, err = goop.chat.create(name, description, max_members)
func chatRoomCreateFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		cm := engine.chatRooms
		if cm == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("chat not available"))
			return 2
		}
		name := L.CheckString(1)
		desc := L.OptString(2, "")
		max := L.OptInt(3, 0)
		context := L.OptString(4, "")
		room, err := cm.CreateRoom(name, desc, context, max)
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

// chatRoomCloseFn closes a chat room.
//
//	err = goop.chat.close(group_id)
func chatRoomCloseFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		cm := engine.chatRooms
		if cm == nil {
			L.Push(lua.LString("chat not available"))
			return 1
		}
		groupID := L.CheckString(1)
		if err := cm.CloseRoom(groupID); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// chatRoomSendFn sends a message to a chat room.
//
//	err = goop.chat.send(group_id, text)
func chatRoomSendFn(inv *invocationCtx, engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		cm := engine.chatRooms
		if cm == nil {
			L.Push(lua.LString("chat not available"))
			return 1
		}
		groupID := L.CheckString(1)
		text := L.CheckString(2)
		if err := cm.SendMessage(groupID, inv.peerID, text); err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}
}

// chatRoomStateFn returns room state with members and recent messages.
//
//	room, err = goop.chat.state(group_id)
func chatRoomStateFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		cm := engine.chatRooms
		if cm == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("chat not available"))
			return 2
		}
		groupID := L.CheckString(1)
		room, msgs, err := cm.GetState(groupID)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		tbl := roomToLua(L, room)
		tbl.RawSetString("messages", messagesToLua(L, msgs))
		L.Push(tbl)
		L.Push(lua.LNil)
		return 2
	}
}

// chatRoomListFn lists active rooms.
//
//	rooms = goop.chat.rooms()
func chatRoomListFn(engine *Engine) lua.LGFunction {
	return func(L *lua.LState) int {
		cm := engine.chatRooms
		if cm == nil {
			L.Push(L.NewTable())
			return 1
		}
		rooms := cm.ListRooms()
		tbl := L.NewTable()
		for i, r := range rooms {
			tbl.RawSetInt(i+1, roomToLua(L, &r))
		}
		L.Push(tbl)
		return 1
	}
}

func roomToLua(L *lua.LState, r *chatType.Room) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("id", lua.LString(r.ID))
	tbl.RawSetString("name", lua.LString(r.Name))
	tbl.RawSetString("description", lua.LString(r.Description))
	if len(r.Members) > 0 {
		members := L.NewTable()
		for i, m := range r.Members {
			entry := L.NewTable()
			entry.RawSetString("peer_id", lua.LString(m.PeerID))
			entry.RawSetString("name", lua.LString(m.Name))
			members.RawSetInt(i+1, entry)
		}
		tbl.RawSetString("members", members)
	}
	return tbl
}

func messagesToLua(L *lua.LState, msgs []chatType.Message) *lua.LTable {
	tbl := L.NewTable()
	for i, m := range msgs {
		entry := L.NewTable()
		entry.RawSetString("id", lua.LString(m.ID))
		entry.RawSetString("from", lua.LString(m.From))
		entry.RawSetString("from_name", lua.LString(m.FromName))
		entry.RawSetString("text", lua.LString(m.Text))
		entry.RawSetString("timestamp", lua.LNumber(m.Timestamp))
		tbl.RawSetInt(i+1, entry)
	}
	return tbl
}
