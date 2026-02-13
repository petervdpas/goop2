package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/petervdpas/goop2/internal/proto"
	"github.com/petervdpas/goop2/internal/storage"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// LuaDispatcher handles lua-call and lua-list data operations.
type LuaDispatcher interface {
	CallFunction(ctx context.Context, callerID, function string, params map[string]any) (any, error)
	ListDataFunctions() any
	RescanFunctions() // reload scripts from the functions directory
}

// DataRequest is the wire format for a data operation request.
type DataRequest struct {
	Op         string              `json:"op"`
	Table      string              `json:"table,omitempty"`
	Name       string              `json:"name,omitempty"`
	Data       map[string]any      `json:"data,omitempty"`
	ID         int64               `json:"id,omitempty"`
	Where      string              `json:"where,omitempty"`
	Args       []any               `json:"args,omitempty"`
	Columns    []string            `json:"columns,omitempty"`
	ColumnDefs []storage.ColumnDef `json:"column_defs,omitempty"`
	Column     *storage.ColumnDef  `json:"column,omitempty"`
	Limit      int                 `json:"limit,omitempty"`
	Offset     int                 `json:"offset,omitempty"`
	OldName    string              `json:"old_name,omitempty"`
	NewName    string              `json:"new_name,omitempty"`
	Function   string              `json:"function,omitempty"` // for lua-call
	Params     map[string]any      `json:"params,omitempty"`  // for lua-call
}

// DataResponse is the wire format for a data operation response.
type DataResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// EnableData stores the DB reference and registers the data stream handler.
func (n *Node) EnableData(db *storage.DB) {
	n.db = db
	n.Host.SetStreamHandler(protocol.ID(proto.DataProtoID), n.handleDataStream)
}

func (n *Node) handleDataStream(s network.Stream) {
	defer s.Close()

	callerID := s.Conn().RemotePeer().String()

	rd := bufio.NewReader(s)
	line, err := rd.ReadBytes('\n')
	if err != nil && err != io.EOF {
		writeDataResponse(s, DataResponse{Error: "read error"})
		return
	}

	var req DataRequest
	if err := json.Unmarshal(line, &req); err != nil {
		writeDataResponse(s, DataResponse{Error: "invalid json: " + err.Error()})
		return
	}

	resp := n.dispatchDataOp(callerID, req)
	writeDataResponse(s, resp)
}

func writeDataResponse(s network.Stream, resp DataResponse) {
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = s.Write(b)
}

func (n *Node) dispatchDataOp(callerID string, req DataRequest) DataResponse {
	if n.db == nil {
		return DataResponse{Error: "database not available"}
	}

	isLocal := callerID == n.ID()

	switch req.Op {
	case "tables":
		return n.dataOpTables()
	case "describe":
		return n.dataOpDescribe(req)
	case "query":
		if isLocal {
			return n.dataOpQueryLocal(req)
		}
		return n.dataOpQuery(callerID, req)
	case "insert":
		return n.dataOpInsert(callerID, req)
	case "update":
		if isLocal {
			return n.dataOpUpdateLocal(req)
		}
		return n.dataOpUpdate(callerID, req)
	case "delete":
		if isLocal {
			return n.dataOpDeleteLocal(req)
		}
		return n.dataOpDelete(callerID, req)
	case "create-table", "add-column", "drop-column", "rename-table", "delete-table":
		if !isLocal {
			return DataResponse{Error: "schema operations not allowed for remote peers"}
		}
		return n.dispatchSchemaOp(req)
	case "lua-call":
		return n.dataOpLuaCall(callerID, req)
	case "lua-list":
		return n.dataOpLuaList()
	default:
		return DataResponse{Error: fmt.Sprintf("unknown op: %s", req.Op)}
	}
}

func (n *Node) dataOpTables() DataResponse {
	tables, err := n.db.ListTables()
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: tables}
}

func (n *Node) dataOpDescribe(req DataRequest) DataResponse {
	table := req.Table
	if table == "" {
		table = req.Name
	}
	if table == "" {
		return DataResponse{Error: "table name required"}
	}
	cols, err := n.db.DescribeTable(table)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: cols}
}

// dataOpQueryLocal is the unrestricted local variant — no _owner scoping.
func (n *Node) dataOpQueryLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	rows, err := n.db.SelectPaged(storage.SelectOpts{
		Table:   req.Table,
		Columns: req.Columns,
		Where:   req.Where,
		Args:    req.Args,
		Limit:   req.Limit,
		Offset:  req.Offset,
	})
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: rows}
}

// dataOpUpdateLocal is the unrestricted local variant — no _owner check.
func (n *Node) dataOpUpdateLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	if req.ID <= 0 {
		return DataResponse{Error: "valid row id required"}
	}
	if err := n.db.UpdateRow(req.Table, req.ID, req.Data); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{"status": "updated"}}
}

// dataOpDeleteLocal is the unrestricted local variant — no _owner check.
func (n *Node) dataOpDeleteLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	if req.ID <= 0 {
		return DataResponse{Error: "valid row id required"}
	}
	if err := n.db.DeleteRow(req.Table, req.ID); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{"status": "deleted"}}
}

// dispatchSchemaOp routes schema-mutating operations (local only).
func (n *Node) dispatchSchemaOp(req DataRequest) DataResponse {
	switch req.Op {
	case "create-table":
		return n.dataOpCreateTable(req)
	case "add-column":
		return n.dataOpAddColumn(req)
	case "drop-column":
		return n.dataOpDropColumn(req)
	case "rename-table":
		return n.dataOpRenameTable(req)
	case "delete-table":
		return n.dataOpDeleteTable(req)
	default:
		return DataResponse{Error: fmt.Sprintf("unknown schema op: %s", req.Op)}
	}
}

// dataOpQuery handles remote read queries. For "owner" and "public" policy
// tables the data is publicly readable, so no _owner scoping is applied.
// For other policies each caller sees only their own rows.
func (n *Node) dataOpQuery(callerID string, req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}

	where := req.Where
	args := req.Args

	// Owner-only and public tables have public reads — skip _owner scoping.
	policy, _ := n.db.GetTableInsertPolicy(req.Table)
	if policy != "owner" && policy != "public" {
		// Scope query to caller's own rows: inject _owner = ? condition
		if where != "" {
			where = "(" + where + ") AND _owner = ?"
			args = append(args, callerID)
		} else {
			where = "_owner = ?"
			args = []any{callerID}
		}
	}

	rows, err := n.db.SelectPaged(storage.SelectOpts{
		Table:   req.Table,
		Columns: req.Columns,
		Where:   where,
		Args:    args,
		Limit:   req.Limit,
		Offset:  req.Offset,
	})
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: rows}
}

func (n *Node) dataOpInsert(callerID string, req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}

	isLocal := callerID == n.ID()

	// _owner is the caller's peer ID (cryptographically authenticated by libp2p)
	ownerEmail := ""
	if isLocal && n.selfEmail != nil {
		// Local peer: use own email from config
		ownerEmail = n.selfEmail()
	} else if sp, ok := n.peers.Get(callerID); ok {
		ownerEmail = sp.Email
	}

	// Enforce per-table insert policy
	policy, _ := n.db.GetTableInsertPolicy(req.Table)
	switch policy {
	case "owner":
		if !isLocal {
			return DataResponse{Error: "insert not allowed: this table only accepts data from the site owner"}
		}
	case "email":
		if !isLocal && ownerEmail == "" {
			return DataResponse{Error: "insert not allowed: your peer must have an email address configured"}
		}
	case "open", "public":
		// anyone can insert
	default:
		// unknown policy — default to owner-only for safety
		if !isLocal {
			return DataResponse{Error: "insert not allowed: unknown table policy"}
		}
	}

	log.Printf("[data] insert into %s by %s (%s) [policy=%s]", req.Table, callerID, ownerEmail, policy)

	id, err := n.db.Insert(req.Table, callerID, ownerEmail, req.Data)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]any{
		"status": "inserted",
		"id":     id,
	}}
}

func (n *Node) dataOpUpdate(callerID string, req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	if req.ID <= 0 {
		return DataResponse{Error: "valid row id required"}
	}
	// Only allow updating rows owned by the caller
	if err := n.db.UpdateRowOwner(req.Table, req.ID, callerID, req.Data); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{"status": "updated"}}
}

func (n *Node) dataOpDelete(callerID string, req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	if req.ID <= 0 {
		return DataResponse{Error: "valid row id required"}
	}
	// Only allow deleting rows owned by the caller
	if err := n.db.DeleteRowOwner(req.Table, req.ID, callerID); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{"status": "deleted"}}
}

func (n *Node) dataOpCreateTable(req DataRequest) DataResponse {
	name := req.Name
	if name == "" {
		name = req.Table
	}
	if name == "" {
		return DataResponse{Error: "table name required"}
	}
	columns := req.ColumnDefs
	if len(columns) == 0 && req.Column != nil {
		columns = []storage.ColumnDef{*req.Column}
	}
	if len(columns) == 0 {
		return DataResponse{Error: "at least one column required"}
	}
	if err := n.db.CreateTable(name, columns); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{
		"status": "created",
		"table":  name,
	}}
}

func (n *Node) dataOpAddColumn(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	if req.Column == nil {
		return DataResponse{Error: "column definition required"}
	}
	if err := n.db.AddColumn(req.Table, *req.Column); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{"status": "added"}}
}

func (n *Node) dataOpDropColumn(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	colName := ""
	if req.Column != nil {
		colName = req.Column.Name
	}
	if colName == "" && req.Name != "" {
		colName = req.Name
	}
	if colName == "" {
		return DataResponse{Error: "column name required"}
	}
	if err := n.db.DropColumn(req.Table, colName); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{"status": "dropped"}}
}

func (n *Node) dataOpRenameTable(req DataRequest) DataResponse {
	if req.OldName == "" || req.NewName == "" {
		return DataResponse{Error: "old and new name required"}
	}
	if err := n.db.RenameTable(req.OldName, req.NewName); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{
		"status":   "renamed",
		"new_name": req.NewName,
	}}
}

func (n *Node) dataOpDeleteTable(req DataRequest) DataResponse {
	table := req.Table
	if table == "" {
		table = req.Name
	}
	if table == "" {
		return DataResponse{Error: "table name required"}
	}
	if err := n.db.DeleteTable(table); err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]string{
		"status": "deleted",
		"table":  table,
	}}
}

func (n *Node) dataOpLuaCall(callerID string, req DataRequest) DataResponse {
	if n.luaDispatcher == nil {
		return DataResponse{Error: "lua scripting not enabled"}
	}
	if req.Function == "" {
		return DataResponse{Error: "function name required"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := n.luaDispatcher.CallFunction(ctx, callerID, req.Function, req.Params)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: result}
}

func (n *Node) dataOpLuaList() DataResponse {
	if n.luaDispatcher == nil {
		return DataResponse{OK: true, Data: map[string]any{"functions": []any{}}}
	}
	return DataResponse{OK: true, Data: map[string]any{"functions": n.luaDispatcher.ListDataFunctions()}}
}

// LocalDataOp executes a data operation on the local database, using callerID as the owner.
func (n *Node) LocalDataOp(callerID string, req DataRequest) DataResponse {
	return n.dispatchDataOp(callerID, req)
}

// RemoteDataOp opens a P2P stream to the given peer and executes a data operation.
func (n *Node) RemoteDataOp(ctx context.Context, peerID string, req DataRequest) (DataResponse, error) {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return DataResponse{}, fmt.Errorf("invalid peer id: %w", err)
	}

	// Best effort connect (mDNS usually already connected)
	_ = n.Host.Connect(ctx, peer.AddrInfo{ID: pid})

	s, err := n.Host.NewStream(ctx, pid, protocol.ID(proto.DataProtoID))
	if err != nil {
		return DataResponse{}, fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()

	// Send request as JSON line
	b, err := json.Marshal(req)
	if err != nil {
		return DataResponse{}, err
	}
	b = append(b, '\n')
	if _, err := s.Write(b); err != nil {
		return DataResponse{}, fmt.Errorf("write request: %w", err)
	}

	// Signal that we're done writing
	_ = s.CloseWrite()

	// Read response
	rd := bufio.NewReader(s)
	respLine, err := rd.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return DataResponse{}, fmt.Errorf("read response: %w", err)
	}

	var resp DataResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return DataResponse{}, fmt.Errorf("decode response: %w", err)
	}

	return resp, nil
}
