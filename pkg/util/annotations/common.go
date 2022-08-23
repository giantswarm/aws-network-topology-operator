package annotations

const (
	// NetworkTopologyModeAnnotation is the annotation indicating the network topology mode a cluster uses
	// Valid values are "GiantSwarmManaged", "CustomerManaged" and "None"
	NetworkTopologyModeAnnotation = "network-topology.giantswarm.io/mode"

	NetworkTopologyModeGiantSwarmManaged = "GiantSwarmManaged"
	NetworkTopologyModeUserManaged       = "UserManaged"
	NetworkTopologyModeNone              = "None"

	// NetworkTopologyTransitGatewayIDAnnotation contains the ID of the Transit Gateway used by the cluster.
	// This is either the user-provided TGW or the one created by this operator.
	NetworkTopologyTransitGatewayIDAnnotation = "network-topology.giantswarm.io/transit-gateway"
)
