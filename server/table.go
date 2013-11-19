/* Copyright (C) 2013 CompleteDB LLC.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with PubSubSQL.  If not, see <http://www.gnu.org/licenses/>.
 */

package pubsubsql

import "strconv"

// Default values
const (
	tableCOLUMNS int = 10
	tableRECORDS     = 5000
)

// this function is purely for testing porposes
func (t *table) getTagedColumnValuesCount(col string, val string) int {
	c := t.getColumn(col)
	if c == nil || !c.isTag() {
		return 0
	}
	i := 0
	for tg := c.tags[val]; tg != nil; tg = tg.next {
		i++
	}
	return i
}

// table  
type table struct {
	name         string
	colMap       map[string]*column
	colSlice     []*column
	records      []*record
	tagedColumns []*column
	keyColumns   []*column
}

// table factory 
func newTable(name string) *table {
	t := &table{
		name:         name,
		colMap:       make(map[string]*column),
		colSlice:     make([]*column, 0, tableCOLUMNS),
		records:      make([]*record, 0, tableRECORDS),
		tagedColumns: make([]*column, 0, tableCOLUMNS),
	}
	t.addColumn("id")
	return t
}

// COLUMNS functions

// Returns total number of columns. 
func (t *table) getColumnCount() int {
	l := len(t.colSlice)
	if l != len(t.colMap) {
		panic("Something bad happened column slice and map do not match")
	}
	return l
}

// Adds column and returns column added column.
func (t *table) addColumn(name string) *column {
	col := newColumn(name, len(t.colSlice))
	t.colMap[name] = col
	t.colSlice = append(t.colSlice, col)
	return col
}

// Tries to retrieve existing column or adds it if does not exist.
// Returns true when new column was added.
func (t *table) getAddColumn(name string) (*column, bool) {
	col, columnExists := t.colMap[name]
	if columnExists {
		return col, false
	}
	return t.addColumn(name), true
}

// Retrieves existing column
func (t *table) getColumn(name string) *column {
	col, ok := t.colMap[name]
	if ok {
		return col
	}
	return nil
}

// RECORDS functions

// newRecord creates new record but does not add it to the table
func (t *table) newRecord() (*record, int) {
	l := len(t.records)
	r := newRecord(len(t.colSlice), strconv.Itoa(l))
	ltags := len(t.tagedColumns)
	if ltags > 0 {
		r.tags = make([]*tag, ltags)
	}
	return r, l
}

// adNewRecord add newly created record to the table
func (t *table) addNewRecord(r *record) {
	addRecordToSlice(&t.records, r)
}

// addRecordToSlice generic helper function that adds record to the slice and
// automatically expands the slice
func addRecordToSlice(records *[]*record, r *record) {
	//check if records slice needs to grow by third 
	l := len(*records)
	if cap(*records) == len(*records) {
		temp := *records
		*records = make([]*record, l, l+(l/3))
		copy(*records, temp)
	}
	*records = append(*records, r)
}

// getRecords returns record by id
func (t *table) getRecord(id int) *record {
	if len(t.records) > id {
		return t.records[id]
	}
	return nil
}

// getRecordCount returns total number of records in the table
func (t *table) getRecordCount() int {
	return len(t.records)
}

// sqlKey defines unique index in the table
func (t *table) sqlKey(req *sqlKeyRequest) response {
	// key is already defined for this column
	col := t.getColumn(req.column)
	if col != nil && col.isIndexed() {
		return newErrorResponse("key or tag already defined for column:" + req.column)
	}
	// new column on existing records
	if col == nil && len(t.records) > 0 {
		return newErrorResponse("can not define key for non existant column due to possible duplicates")
	}
	key := make(map[string]int, cap(t.records))
	// new column no records
	if col == nil {
		col, _ = t.getAddColumn(req.column)
	} else { // existing column
		// index all records and check if there are duplicates
		key = make(map[string]int, cap(t.records))
		for idx, rec := range t.records {
			val := rec.getValue(col.ordinal)
			if col.keyContainsValue(val) {
				return newErrorResponse("can not define key due to possible duplicates in existing records")
			}
			key[val] = idx
		}
	}
	//
	col.makeKey(key)
	t.keyColumns = append(t.keyColumns, col)
	//
	return newOkResponse()
}

func addValueToTags(tags map[string]*tag, val string, idx int) *tag {
	head := tags[val]
	tg := addTag(head, idx)
	if head == nil {
		tags[val] = tg
	}
	return tg
}

func (t *table) tagValue(col *column, idx int, rec *record) {
	val := rec.getValue(col.ordinal)
	tg := addValueToTags(col.tags, val, idx)
	if len(rec.tags) <= col.tagIndex {
		rec.tags = append(rec.tags, tg)
	} else {
		rec.tags[col.tagIndex] = tg
	}
}

func (t *table) sqlTag(req *sqlTagRequest) response {
	// tag is already defined for this column
	col := t.getColumn(req.column)
	if col != nil && col.isIndexed() {
		return newErrorResponse("key or tag already defined for column:" + req.column)
	}
	col, _ = t.getAddColumn(req.column)
	t.tagedColumns = append(t.tagedColumns, col)
	col.makeTags(len(t.tagedColumns) - 1)
	// tag existing values
	for idx, rec := range t.records {
		t.tagValue(col, idx, rec)
	}
	return newOkResponse()
}

// removeColumns removes columns starting at particular ordinal
func (t *table) removeColumns(ordinal int) {
	if len(t.colSlice) <= ordinal {
		return
	}
	tail := t.colSlice[ordinal:]
	for _, col := range tail {
		delete(t.colMap, col.name)
	}
	t.colSlice = t.colSlice[:ordinal]
}

func (t *table) insertRecord(cols []*column, colVals []*columnValue, rec *record, id int) {
	for idx, colVal := range colVals {
		col := cols[idx]
		rec.setValue(col.ordinal, colVal.val)
		// update key
		switch col.typ {
		case columnTypeKey:
			col.key[colVal.val] = id
		case columnTypeTag:
			t.tagValue(col, id, rec)
		}
	}
}

// sqlInsert proceses sql insert request and returns response
func (t *table) sqlInsert(req *sqlInsertRequest) response {
	rec, id := t.newRecord()
	// validate unique keys constrain
	cols := make([]*column, len(req.colVals))
	originalColLen := len(t.colSlice)
	for idx, colVal := range req.colVals {
		col, _ := t.getAddColumn(colVal.col)
		if col.isKey() && col.keyContainsValue(colVal.val) {
			//remove created columns
			t.removeColumns(originalColLen)
			return newErrorResponse("insert failed due to duplicate column key:" + colVal.col + " value:" + colVal.val)
		}
		cols[idx] = col
	}
	// ready to insert	
	t.insertRecord(cols, req.colVals, rec, id)
	t.addNewRecord(rec)
	res := sqlInsertResponse{id: rec.getId()}
	return &res
}

func (t *table) getRecordById(val string) []*record {
	idx, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return nil
	}
	records := make([]*record, 1, 1)
	records[0] = t.records[idx]
	return records
}

func (t *table) getRecordByKey(val string, col *column) []*record {
	idx, present := col.key[val]
	if !present {
		return nil
	}
	records := make([]*record, 1, 1)
	records[0] = t.records[idx]
	return records
}

func (t *table) getRecordsByTag(val string, col *column) []*record {
	// we need to optimize allocations
	// perhaps its possible to know in advance how manny records
	// will be returned
	records := make([]*record, 0, 100)
	for tg := col.tags[val]; tg != nil; tg = tg.next {
		records = append(records, t.records[tg.idx])
		l := len(records)
		if cap(records) == l {
			temp := records
			records = make([]*record, l, l+(l/3))
			copy(records, temp)
		}
	}
	return records
}

// returns error response
func (t *table) getRecordsBySqlFilter(filter sqlFilter) ([]*record, response) {
	var col *column
	if len(filter.col) > 0 {
		col = t.getColumn(filter.col)
		if col == nil {
			return nil, newErrorResponse("invalid column: " + filter.col)
		}
	}
	if col == nil {
		// all
		return t.records, nil
	}
	switch col.typ {
	case columnTypeId:
		return t.getRecordById(filter.val), nil
	case columnTypeKey:
		return t.getRecordByKey(filter.val, col), nil
	case columnTypeTag:
		return t.getRecordsByTag(filter.val, col), nil
	}
	return nil, newErrorResponse("can not use non indexed column " + filter.col + " as valid filter")
}

func (t *table) sqlSelect(req *sqlSelectRequest) response {
	records, errResponse := t.getRecordsBySqlFilter(req.filter)
	if errResponse != nil {
		return errResponse
	}
	res := sqlSelectResponse{
		columns: t.colSlice,
		records: make([]*record, 0, len(records)),
	}
	for _, rec := range records {
		if rec != nil {
			res.copyRecordData(rec)
		}
	}
	return &res
}

// UPDATE

func (t *table) updateRecord(cols []*column, colVals []*columnValue, rec *record, id int) {
	for idx, colVal := range colVals {
		col := cols[idx]
		switch col.typ {
		case columnTypeKey:
			// delete previous key
			delete(col.key, rec.getValue(col.ordinal))
			rec.setValue(col.ordinal, colVal.val)
			col.key[colVal.val] = id
		case columnTypeTag:
			// delete previous tag
			t.deleteTag(rec, col)
			rec.setValue(col.ordinal, colVal.val)
			t.tagValue(col, id, rec)
		case columnTypeNormal:
			rec.setValue(col.ordinal, colVal.val)
		}
	}
}

func (t *table) sqlUpdate(req *sqlUpdateRequest) response {
	records, errResponse := t.getRecordsBySqlFilter(req.filter)
	if errResponse != nil {
		return errResponse
	}
	res := &sqlUpdateResponse{updated: 0}
	var onlyRecord *record
	switch len(records) {
	case 0:
		return res
	case 1:
		onlyRecord = records[0]
	}
	// validate duplicate keys
	cols := make([]*column, len(req.colVals))
	originalColLen := len(t.colSlice)
	for idx, colVal := range req.colVals {
		col, _ := t.getAddColumn(colVal.col)
		if col.isKey() && col.keyContainsValue(colVal.val) {
			if onlyRecord == nil || onlyRecord != t.getRecordByKey(colVal.val, col)[0] {
				//remove created columns
				t.removeColumns(originalColLen)
				return newErrorResponse("update failed due to duplicate column key:" + colVal.col + " value:" + colVal.val)
			}
		}
		cols[idx] = col
	}
	// all is valid ready to update
	updated := 0
	for _, rec := range records {
		if rec != nil {
			t.updateRecord(cols, req.colVals, rec, int(rec.idx()))
			updated++
		}
	}
	res.updated = updated
	return res
}

// DELETE

func (t *table) deleteTag(rec *record, col *column) {
	tg := rec.tags[col.tagIndex]
	rec.tags[col.tagIndex] = nil
	switch removeTag(tg) {
	case removeTagLast:
		delete(col.tags, rec.getValue(col.ordinal))
	case removeTagSlide:
		// we need to retag the slided record	
		slidedRecord := t.records[tg.idx]
		if slidedRecord != nil {
			slidedRecord.tags[col.tagIndex] = tg
		}
	}
}

func (t *table) deleteRecord(rec *record) {
	// delete record keys
	for _, col := range t.keyColumns {
		delete(col.key, rec.getValue(col.ordinal))
	}
	// delete record tags
	for _, col := range t.tagedColumns {
		t.deleteTag(rec, col)
	}
	// delete record
	t.records[rec.idx()] = nil
}

func (t *table) sqlDelete(req *sqlDeleteRequest) response {
	records, errResponse := t.getRecordsBySqlFilter(req.filter)
	if errResponse != nil {
		return errResponse
	}
	deleted := 0
	for _, rec := range records {
		if rec != nil {
			deleted++
			t.deleteRecord(rec)
		}
	}
	return &sqlDeleteResponse{deleted: deleted}
}