package schema

// IntersectColumns returns columns that appear in all provided tables
// with the same name and type. Key/Required/Auto flags are kept from the
// first table. Returns nil if there are no common columns.
func IntersectColumns(tables []Table) []Column {
	if len(tables) == 0 {
		return nil
	}
	if len(tables) == 1 {
		cols := make([]Column, len(tables[0].Columns))
		copy(cols, tables[0].Columns)
		return cols
	}

	type colSig struct {
		Name string
		Type string
	}

	counts := make(map[colSig]int)
	first := make(map[colSig]Column)

	for _, col := range tables[0].Columns {
		sig := colSig{Name: col.Name, Type: col.Type}
		counts[sig] = 1
		first[sig] = col
	}

	for _, tbl := range tables[1:] {
		seen := make(map[colSig]bool)
		for _, col := range tbl.Columns {
			sig := colSig{Name: col.Name, Type: col.Type}
			if _, exists := counts[sig]; exists && !seen[sig] {
				counts[sig]++
				seen[sig] = true
			}
		}
	}

	n := len(tables)
	var result []Column
	for _, col := range tables[0].Columns {
		sig := colSig{Name: col.Name, Type: col.Type}
		if counts[sig] == n {
			result = append(result, first[sig])
		}
	}
	return result
}

// MergeTable creates a merged table from multiple tables with the same name,
// using only the intersection of their columns. Returns nil if no common
// columns exist or tables have different names.
func MergeTable(name string, tables []Table) *Table {
	cols := IntersectColumns(tables)
	if len(cols) == 0 {
		return nil
	}

	hasKey := false
	for _, c := range cols {
		if c.Key {
			hasKey = true
			break
		}
	}
	if !hasKey {
		return nil
	}

	return &Table{
		Name:    name,
		Columns: cols,
		Context: true,
	}
}
