package province

// DefaultGoodStorageCap is the finite technical ceiling used when creating a
// settlement_goods row. It is shared by founding, production and cargo credits.
// Existing rows keep their own cap. This is the current flat placeholder, not
// the future storage-pool model (megaron_plan_lagerpooler).
const DefaultGoodStorageCap = 1_000_000.0
