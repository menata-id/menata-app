package domain

// TableAlign is which edge a Table column's text sits on, as a closed set -- a semantic ("numbers end-align"),
// never a class (007 §15.2). Start is the default for words and End is for figures, so a column of counts reads
// down its units digits; a Theme may move what either draws and a page never names a class.
type TableAlign string

const (
	TableAlignStart TableAlign = "start"
	TableAlignEnd   TableAlign = "end"
)

// KnownTableAligns is the closed set.
var KnownTableAligns = map[TableAlign]bool{TableAlignStart: true, TableAlignEnd: true}
