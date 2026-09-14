package dataplane

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"

	"github.com/wraplink/edge-agent/internal/client"
)

const (
	tableName = "wraplink"
	chainName = "prerouting"
)

type NFT struct {
	mu sync.Mutex

	conn  *nftables.Conn
	table *nftables.Table
	chain *nftables.Chain

	routes map[string]*routeState
}

type routeState struct {
	lease client.Lease
	rule  *nftables.Rule
}

func NewNFT() (*NFT, error) {
	conn := &nftables.Conn{}

	table := conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   tableName,
	})

	chain := conn.AddChain(&nftables.Chain{
		Table:    table,
		Name:     chainName,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPrerouting,
		Priority: nftables.ChainPriorityNATDest,
	})

	if err := conn.Flush(); err != nil {
		return nil, fmt.Errorf("create nftables base: %w", err)
	}

	return &NFT{
		conn:   conn,
		table:  table,
		chain:  chain,
		routes: make(map[string]*routeState),
	}, nil
}

func (n *NFT) AddRoute(
	ctx context.Context,
	lease client.Lease,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateLease(lease); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Already installed.
	if _, exists := n.routes[lease.ID]; exists {
		return nil
	}

	edgeIP := net.ParseIP(lease.Address).To4()
	destinationIP := net.ParseIP(lease.DestinationIP).To4()

	if edgeIP == nil {
		return fmt.Errorf("invalid edge IPv4 address %q", lease.Address)
	}

	if destinationIP == nil {
		return fmt.Errorf(
			"invalid destination IPv4 address %q",
			lease.DestinationIP,
		)
	}

	protocol, err := protocolNumber(lease.Protocol)
	if err != nil {
		return err
	}

	portStart, portEnd := destinationPortRange(lease)

	exprs := []expr.Any{
		// IPv4 destination address.
		//
		// IPv4 header:
		//   source      = offset 12
		//   destination = offset 16
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       16,
			Len:          4,
		},

		// Match edge IP.
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     edgeIP,
		},

		// Match L4 protocol.
		&expr.Meta{
			Key:      expr.MetaKeyL4PROTO,
			Register: 1,
		},

		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{protocol},
		},
	}

	// Destination port.
	//
	// TCP/UDP header:
	//   source port      = offset 0
	//   destination port = offset 2
	//
	// If the lease specifies a range, match it.
	if portStart != 0 || portEnd != 0 {
		exprs = append(exprs,
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       2,
				Len:          2,
			},

			&expr.Cmp{
				Op:       expr.CmpOpGte,
				Register: 1,
				Data:     binaryutil.BigEndian.PutUint16(portStart),
			},

			&expr.Cmp{
				Op:       expr.CmpOpLte,
				Register: 1,
				Data:     binaryutil.BigEndian.PutUint16(portEnd),
			},
		)
	}

	// Destination IP for DNAT.
	exprs = append(exprs,
		&expr.Immediate{
			Register: 1,
			Data:     destinationIP,
		},
	)

	// Destination port.
	//
	// If no destination port range is configured, preserve the
	// original destination port.
	if portStart != 0 || portEnd != 0 {
		exprs = append(exprs,
			&expr.Immediate{
				Register: 2,
				Data:     binaryutil.BigEndian.PutUint16(portStart),
			},
		)
	}

	nat := &expr.NAT{
		Type:       expr.NATTypeDestNAT,
		Family:     uint32(nftables.TableFamilyIPv4),
		RegAddrMin: 1,
	}

	if portStart != 0 || portEnd != 0 {
		nat.RegProtoMin = 2

		if portEnd != portStart {
			// For a port range, load the upper bound into register 3.
			exprs = append(exprs,
				&expr.Immediate{
					Register: 3,
					Data:     binaryutil.BigEndian.PutUint16(portEnd),
				},
			)

			nat.RegProtoMax = 3
		}
	}

	exprs = append(exprs, nat)

	rule := &nftables.Rule{
		Table: n.table,
		Chain: n.chain,
		Exprs: exprs,
	}

	n.conn.AddRule(rule)

	if err := n.conn.Flush(); err != nil {
		return fmt.Errorf("add route %s: %w", lease.ID, err)
	}

	n.routes[lease.ID] = &routeState{
		lease: lease,
		rule:  rule,
	}

	return nil
}

func (n *NFT) RemoveRoute(
	ctx context.Context,
	lease client.Lease,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	state, exists := n.routes[lease.ID]
	if !exists {
		return nil
	}

	if state.rule == nil || state.rule.Handle == 0 {
		delete(n.routes, lease.ID)
		return nil
	}

	if err := n.conn.DelRule(state.rule); err != nil {
		return fmt.Errorf(
			"delete route %s: %w",
			lease.ID,
			err,
		)
	}

	if err := n.conn.Flush(); err != nil {
		return fmt.Errorf(
			"flush route deletion %s: %w",
			lease.ID,
			err,
		)
	}

	delete(n.routes, lease.ID)

	return nil
}

func validateLease(lease client.Lease) error {
	if lease.ID == "" {
		return fmt.Errorf("lease id is required")
	}

	if lease.Address == "" {
		return fmt.Errorf("lease address is required")
	}

	if lease.DestinationIP == "" {
		return fmt.Errorf("destination ip is required")
	}

	edgeIP := net.ParseIP(lease.Address).To4()
	if edgeIP == nil {
		return fmt.Errorf(
			"invalid edge IPv4 address %q",
			lease.Address,
		)
	}

	destinationIP := net.ParseIP(lease.DestinationIP).To4()
	if destinationIP == nil {
		return fmt.Errorf(
			"invalid destination IPv4 address %q",
			lease.DestinationIP,
		)
	}

	if lease.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}

	switch lease.Protocol {
	case "tcp", "udp":
	default:
		return fmt.Errorf(
			"unsupported protocol %q",
			lease.Protocol,
		)
	}

	start, end := destinationPortRange(lease)

	if start == 0 && end == 0 {
		return nil
	}

	if start == 0 || end == 0 {
		return fmt.Errorf(
			"destination port range must specify both start and end",
		)
	}

	if start > end {
		return fmt.Errorf(
			"destination port range is invalid: %d-%d",
			start,
			end,
		)
	}

	return nil
}

func protocolNumber(protocol string) (byte, error) {
	switch protocol {
	case "tcp":
		return unix.IPPROTO_TCP, nil

	case "udp":
		return unix.IPPROTO_UDP, nil

	default:
		return 0, fmt.Errorf(
			"unsupported protocol %q",
			protocol,
		)
	}
}

func destinationPortRange(
	lease client.Lease,
) (uint16, uint16) {
	start := lease.DestinationPortStart
	end := lease.DestinationPortEnd

	// No range means all ports.
	if start == 0 && end == 0 {
		return 0, 0
	}

	return start, end
}
