package model

import (
	"time"

	"gorm.io/gorm"
)

// Timestamps is the audit / soft-delete fieldset for models that use a
// custom (string / UUID) primary key.
//
// It is the equivalent of `gorm.Model` minus the embedded `ID uint`.
// Embedding `gorm.Model` in a struct that already declares
// `ID string \`gorm:"primaryKey;type:uuid"\“ leaves two fields named `ID`
// in the schema mapping (string + uint).  Some GORM operations
// (notably INSERT ... RETURNING on PostgreSQL) then return the same `id`
// column twice and try to scan the UUID value into the embedded uint,
// producing errors of the form:
//
//	sql: Scan error on column index 1, name "id":
//	    converting driver.Value type string ("…") to a uint: invalid syntax
//
// All UUID-keyed models must use Timestamps in place of gorm.Model.
type Timestamps struct {
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
