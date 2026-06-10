package controller

// Condition types and reasons shared across all Forail CR controllers.
const (
	conditionReady  = "Ready"
	conditionSynced = "Synced"

	reasonReconciling = "Reconciling"
	reasonResolveErr  = "ResolveError"
	reasonAPIError    = "ForailAPIError"
	reasonInSync      = "InSync"
)
