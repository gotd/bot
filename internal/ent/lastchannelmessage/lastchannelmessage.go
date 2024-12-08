// Code generated by ent, DO NOT EDIT.

package lastchannelmessage

import (
	"entgo.io/ent/dialect/sql"
)

const (
	// Label holds the string label denoting the lastchannelmessage type in the database.
	Label = "last_channel_message"
	// FieldID holds the string denoting the id field in the database.
	FieldID = "id"
	// FieldMessageID holds the string denoting the message_id field in the database.
	FieldMessageID = "message_id"
	// Table holds the table name of the lastchannelmessage in the database.
	Table = "last_channel_messages"
)

// Columns holds all SQL columns for lastchannelmessage fields.
var Columns = []string{
	FieldID,
	FieldMessageID,
}

// ValidColumn reports if the column name is valid (part of the table columns).
func ValidColumn(column string) bool {
	for i := range Columns {
		if column == Columns[i] {
			return true
		}
	}
	return false
}

// OrderOption defines the ordering options for the LastChannelMessage queries.
type OrderOption func(*sql.Selector)

// ByID orders the results by the id field.
func ByID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldID, opts...).ToFunc()
}

// ByMessageID orders the results by the message_id field.
func ByMessageID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldMessageID, opts...).ToFunc()
}