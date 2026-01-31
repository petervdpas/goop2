// internal/p2p/data.go
package p2p

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"goop/internal/proto"
	"goop/internal/storage"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// DataRequest is the wire format for a data operation request.
type DataRequest struct {
	Op         string                 `json:"op"`
	Table      string                 `json:"table,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	ID         int64                  `json:"id,omitempty"`
	Where      string                 `json:"where,omitempty"`
	Args       []interface{}          `json:"args,omitempty"`
	Columns    []string               `json:"columns,omitempty"`
	ColumnDefs []storage.ColumnDef    `json:"column_defs,omitempty"`
	Column     *storage.ColumnDef     `json:"column,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
	Offset     int                    `json:"offset,omitempty"`
	OldName    string                 `json:"old_name,omitempty"`
	NewName    string                 `json:"new_name,omitempty"`
}

// DataResponse is the wire format for a data operation response.
type DataResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
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

	switch req.Op {
	case "tables":
		return n.dataOpTables()
	case "describe":
		return n.dataOpDescribe(req)
	case "query":
		return n.dataOpQuery(req)
	case "insert":
		return n.dataOpInsert(callerID, req)
	case "update":
		return n.dataOpUpdate(req)
	case "delete":
		return n.dataOpDelete(req)
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

func (n *Node) dataOpQuery(req DataRequest) DataResponse {
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

func (n *Node) dataOpInsert(callerID string, req DataRequest) DataResponse {
	if req.Table == "" {
		return DataResponse{Error: "table name required"}
	}

	// _owner is the caller's peer ID (cryptographically authenticated by libp2p)
	ownerEmail := ""
	if sp, ok := n.peers.Get(callerID); ok {
		ownerEmail = sp.Email
	}

	log.Printf("[data] insert into %s by %s (%s)", req.Table, callerID, ownerEmail)

	id, err := n.db.Insert(req.Table, callerID, ownerEmail, req.Data)
	if err != nil {
		return DataResponse{Error: err.Error()}
	}
	return DataResponse{OK: true, Data: map[string]interface{}{
		"status": "inserted",
		"id":     id,
	}}
}

func (n *Node) dataOpUpdate(req DataRequest) DataResponse {
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

func (n *Node) dataOpDelete(req DataRequest) DataResponse {
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
