package lua

import (
	"context"

	"github.com/petervdpas/goop2/internal/content"

	golua "github.com/yuin/gopher-lua"
)

func siteReadFn(cs *content.Store) golua.LGFunction {
	return func(L *golua.LState) int {
		if cs == nil {
			L.Push(golua.LNil)
			L.Push(golua.LString("site not available"))
			return 2
		}
		path := L.CheckString(1)
		data, _, err := cs.Read(context.Background(), path)
		if err != nil {
			L.Push(golua.LNil)
			L.Push(golua.LString(err.Error()))
			return 2
		}
		L.Push(golua.LString(string(data)))
		L.Push(golua.LNil)
		return 2
	}
}
