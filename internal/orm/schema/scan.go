package schema

import "database/sql"

// Scanners bridge sql.Scan → Row map entries with proper type handling.
// Each scanner writes its typed value into the Row map on Scan.

type textScanner struct {
	row Row
	key string
}

func (s *textScanner) Scan(src any) error {
	var v sql.NullString
	if err := v.Scan(src); err != nil {
		return err
	}
	if v.Valid {
		s.row[s.key] = v.String
	} else {
		s.row[s.key] = nil
	}
	return nil
}

type intScanner struct {
	row Row
	key string
}

func (s *intScanner) Scan(src any) error {
	var v sql.NullInt64
	if err := v.Scan(src); err != nil {
		return err
	}
	if v.Valid {
		s.row[s.key] = v.Int64
	} else {
		s.row[s.key] = nil
	}
	return nil
}

type floatScanner struct {
	row Row
	key string
}

func (s *floatScanner) Scan(src any) error {
	var v sql.NullFloat64
	if err := v.Scan(src); err != nil {
		return err
	}
	if v.Valid {
		s.row[s.key] = v.Float64
	} else {
		s.row[s.key] = nil
	}
	return nil
}

type blobScanner struct {
	row Row
	key string
}

func (s *blobScanner) Scan(src any) error {
	if src == nil {
		s.row[s.key] = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		cp := make([]byte, len(v))
		copy(cp, v)
		s.row[s.key] = cp
	default:
		s.row[s.key] = src
	}
	return nil
}
