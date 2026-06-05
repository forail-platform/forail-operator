package controller

// Condition types and reasons shared across all Forge CR controllers.
const (
	conditionReady  = "Ready"
	conditionSynced = "Synced"

	reasonReconciling = "Reconciling"
	reasonResolveErr  = "ResolveError"
	reasonAPIError    = "ForgeAPIError"
	reasonInSync      = "InSync"
)
