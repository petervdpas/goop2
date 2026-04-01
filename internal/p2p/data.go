package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/petervdpas/goop2/internal/orm/schema"
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
	Order      string              `json:"order,omitempty"`
	Fields     []string            `json:"fields,omitempty"`
	Expr       string              `json:"expr,omitempty"`    // for aggregate
	GroupBy    string              `json:"group_by,omitempty"`
	KeyCol     string              `json:"key_col,omitempty"` // for upsert
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

	// Decrypt if encrypted: ENC:base64\n
	jsonLine := line
	if n.enc != nil && len(line) > 4 && string(line[:4]) == "ENC:" {
		trimmed := strings.TrimSpace(string(line[4:]))
		plaintext, err := n.enc.Open(callerID, trimmed)
		if err != nil {
			writeDataResponse(s, DataResponse{Error: "decrypt error"})
			return
		}
		jsonLine = plaintext
	}

	var req DataRequest
	if err := json.Unmarshal(jsonLine, &req); err != nil {
		writeDataResponse(s, DataResponse{Error: "invalid json: " + err.Error()})
		return
	}

	resp := n.dispatchDataOp(callerID, req)
	n.writeDataResponseEnc(s, callerID, resp)
}

func writeDataResponse(s network.Stream, resp DataResponse) {
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = s.Write(b)
}

// writeDataResponseEnc writes the response, encrypting it for the remote peer if possible.
func (n *Node) writeDataResponseEnc(s network.Stream, peerID string, resp DataResponse) {
	b, _ := json.Marshal(resp)
	if n.enc != nil {
		if sealed, err := n.enc.Seal(peerID, b); err == nil {
			_, _ = s.Write([]byte("ENC:" + sealed + "\n"))
			return
		}
	}
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
	case "query-one":
		if isLocal {
			return n.dataOpQueryOneLocal(req)
		}
		return DataResponse{Error: "query-one not available for remote peers yet"}
	case "exists":
		if isLocal {
			return n.dataOpExistsLocal(req)
		}
		return DataResponse{Error: "exists not available for remote peers yet"}
	case "count":
		if isLocal {
			return n.dataOpCountLocal(req)
		}
		return DataResponse{Error: "count not available for remote peers yet"}
	case "pluck":
		if isLocal {
			return n.dataOpPluckLocal(req)
		}
		return DataResponse{Error: "pluck not available for remote peers yet"}
	case "distinct":
		if isLocal {
			return n.dataOpDistinctLocal(req)
		}
		return DataResponse{Error: "distinct not available for remote peers yet"}
	case "aggregate":
		if isLocal {
			return n.dataOpAggregateLocal(req)
		}
		return DataResponse{Error: "aggregate not available for remote peers yet"}
	case "update-where":
		if !isLocal {
			return DataResponse{Error: "update-where not allowed for remote peers"}
		}
		return n.dataOpUpdateWhereLocal(req)
	case "delete-where":
		if !isLocal {
			return DataResponse{Error: "delete-where not allowed for remote peers"}
		}
		return n.dataOpDeleteWhereLocal(req)
	case "upsert":
		if !isLocal {
			return DataResponse{Error: "upsert not allowed for remote peers"}
		}
		return n.dataOpUpsertLocal(callerID, req)
	case "lua-call":
		return n.dataOpLuaCall(callerID, req)
	case "lua-list":
		return n.dataOpLuaList()
	default:
		return DataResponse{Error: fmt.Sprintf("unknown op: %s", req.Op)}
	}
}

func (n *Node) getAccess(table string) schema.Access {
	return n.db.GetAccess(table)
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
	cols := req.Columns
	if len(cols) == 0 {
		cols = req.Fields
	}
	rows, err := n.db.SelectPaged(storage.SelectOpts{
		Table:   req.Table,
		Columns: cols,
		Where:   req.Where,
		Args:    req.Args,
		Order:   req.Order,
		Limit:   req.Limit,
		Offset:  req.Offset,
	})
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: rows}
}

func (n *Node) dataOpQueryOneLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	cols := req.Columns
	if len(cols) == 0 {
		cols = req.Fields
	}
	rows, err := n.db.SelectPaged(storage.SelectOpts{
		Table:   req.Table,
		Columns: cols,
		Where:   req.Where,
		Args:    req.Args,
		Limit:   1,
	})
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	if len(rows) == 0 {
		return DataResponse{OK: true, Data: nil}
	}
	return DataResponse{OK: true, Data: rows[0]}
}

func (n *Node) dataOpExistsLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	rows, err := n.db.SelectPaged(storage.SelectOpts{
		Table:   req.Table,
		Columns: []string{"1"},
		Where:   req.Where,
		Args:    req.Args,
		Limit:   1,
	})
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]bool{"exists": len(rows) > 0}}
}

func (n *Node) dataOpCountLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	rows, err := n.db.Aggregate(req.Table, "COUNT(*) as n", req.Where, req.Args...)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	n64 := int64(0)
	if len(rows) > 0 {
		if v, ok := rows[0]["n"].(int64); ok {
			n64 = v
		}
	}
	return DataResponse{OK: true, Data: map[string]int64{"count": n64}}
}

func (n *Node) dataOpPluckLocal(req DataRequest) DataResponse {
	if req.Table == "" || req.KeyCol == "" && len(req.Fields) == 0 {
		return DataResponse{Error: "table and column required"}
	}
	col := req.KeyCol
	if col == "" && len(req.Fields) > 0 {
		col = req.Fields[0]
	}
	rows, err := n.db.SelectPaged(storage.SelectOpts{
		Table:   req.Table,
		Columns: []string{col},
		Where:   req.Where,
		Args:    req.Args,
		Order:   req.Order,
		Limit:   req.Limit,
	})
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	vals := make([]any, len(rows))
	for i, row := range rows {
		vals[i] = row[col]
	}
	return DataResponse{OK: true, Data: vals}
}

func (n *Node) dataOpDistinctLocal(req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}
	col := req.KeyCol
	if col == "" && len(req.Fields) > 0 {
		col = req.Fields[0]
	}
	if col == "" {
		return DataResponse{Error: "column required"}
	}
	vals, err := n.db.Distinct(req.Table, col, req.Where, req.Args...)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	if vals == nil {
		vals = []any{}
	}
	return DataResponse{OK: true, Data: vals}
}

func (n *Node) dataOpAggregateLocal(req DataRequest) DataResponse {
	if req.Table == "" || req.Expr == "" {
		return DataResponse{Error: "table and expr required"}
	}
	var rows []map[string]any
	var err error
	if req.GroupBy != "" {
		rows, err = n.db.AggregateGroupBy(req.Table, req.Expr, req.GroupBy, req.Where, req.Args...)
	} else {
		rows, err = n.db.Aggregate(req.Table, req.Expr, req.Where, req.Args...)
	}
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return DataResponse{OK: true, Data: rows}
}

func (n *Node) dataOpUpdateWhereLocal(req DataRequest) DataResponse {
	if req.Table == "" || req.Where == "" {
		return DataResponse{Error: "table and where clause required"}
	}
	affected, err := n.db.UpdateWhere(req.Table, req.Data, req.Where, req.Args...)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]int64{"affected": affected}}
}

func (n *Node) dataOpDeleteWhereLocal(req DataRequest) DataResponse {
	if req.Table == "" || req.Where == "" {
		return DataResponse{Error: "table and where clause required"}
	}
	affected, err := n.db.DeleteWhere(req.Table, req.Where, req.Args...)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]int64{"affected": affected}}
}

func (n *Node) dataOpUpsertLocal(callerID string, req DataRequest) DataResponse {
	if req.Table == "" || req.KeyCol == "" {
		return DataResponse{Error: "table and key_col required"}
	}
	email := ""
	if callerID == n.ID() {
		email = n.selfEmail()
	}
	id, err := n.db.Upsert(req.Table, req.KeyCol, callerID, email, req.Data)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]any{"status": "ok", "id": id}}
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

// dataOpQuery handles remote read queries, enforcing the table's read access policy.
func (n *Node) dataOpQuery(callerID string, req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}

	access := n.getAccess(req.Table)

	switch access.Read {
	case "local":
		return DataResponse{Error: "query not allowed: table is local-only"}
	case "group":
		if n.groupChecker == nil || !n.groupChecker.IsTemplateMember(callerID) {
			return DataResponse{Error: "query not allowed: not a group member"}
		}
	case "open":
		// no restriction
	default:
		// "owner" or unrecognized — scope to caller's own rows
	}

	where := req.Where
	args := req.Args

	if access.Read == "owner" || access.Read == "" {
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

	ownerEmail := ""
	if isLocal && n.selfEmail != nil {
		ownerEmail = n.selfEmail()
	} else if sp, ok := n.peers.Get(callerID); ok {
		ownerEmail = sp.Email
	}

	access := n.getAccess(req.Table)

	switch access.Insert {
	case "local":
		if !isLocal {
			return DataResponse{Error: "insert not allowed: table is local-only"}
		}
	case "owner":
		if !isLocal {
			return DataResponse{Error: "insert not allowed: this table only accepts data from the site owner"}
		}
	case "email":
		if !isLocal && ownerEmail == "" {
			return DataResponse{Error: "insert not allowed: your peer must have an email address configured"}
		}
	case "group":
		if !isLocal {
			if n.groupChecker == nil || !n.groupChecker.IsTemplateMember(callerID) {
				return DataResponse{Error: "insert not allowed: not a template group co-author"}
			}
		}
	case "open":
		// anyone can insert
	default:
		if !isLocal {
			return DataResponse{Error: "insert not allowed: unknown table policy"}
		}
	}

	log.Printf("[data] insert into %s by %s (%s) [policy=%s]", req.Table, callerID, ownerEmail, access.Insert)

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
	access := n.getAccess(req.Table)
	if access.Update == "local" {
		return DataResponse{Error: "update not allowed: table is local-only"}
	}
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
	access := n.getAccess(req.Table)
	if access.Delete == "local" {
		return DataResponse{Error: "delete not allowed: table is local-only"}
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), DataLuaCallTimeout)
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

	s, err := n.Host.NewStream(network.WithAllowLimitedConn(ctx, "relay"), pid, protocol.ID(proto.DataProtoID))
	if err != nil {
		return DataResponse{}, fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()

	// Send request as JSON line (encrypted if possible)
	b, err := json.Marshal(req)
	if err != nil {
		return DataResponse{}, err
	}
	if n.enc != nil {
		if sealed, err := n.enc.Seal(peerID, b); err == nil {
			b = []byte("ENC:" + sealed)
		}
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

	// Decrypt response if encrypted
	jsonLine := respLine
	if n.enc != nil && len(respLine) > 4 && string(respLine[:4]) == "ENC:" {
		trimmed := strings.TrimSpace(string(respLine[4:]))
		if plaintext, err := n.enc.Open(peerID, trimmed); err == nil {
			jsonLine = plaintext
		}
	}

	var resp DataResponse
	if err := json.Unmarshal(jsonLine, &resp); err != nil {
		return DataResponse{}, fmt.Errorf("decode response: %w", err)
	}

	return resp, nil
}
