package dialer

import (
	"context"
	"net"
	"net/netip"
	"time"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/deprecated"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

func New(ctx context.Context, options option.DialerOptions, remoteIsDomain bool) (N.Dialer, error) {
	if options.IsWireGuardListener {
		return NewDefault(ctx, options)
	}
	var (
		dialer N.Dialer
		err    error
	)
	if options.Detour == "" {
		dialer, err = NewDefault(ctx, options)
		if err != nil {
			return nil, err
		}
	} else {
		outboundManager := service.FromContext[adapter.OutboundManager](ctx)
		if outboundManager == nil {
			return nil, E.New("missing outbound manager")
		}
		dialer = NewDetour(outboundManager, options.Detour)
	}
	if remoteIsDomain && options.Detour == "" && options.DomainResolver == "" {
		deprecated.Report(ctx, deprecated.OptionMissingDomainResolverInDialOptions)
	}
	if (options.Detour == "" && remoteIsDomain) || options.DomainResolver != "" {
		router := service.FromContext[adapter.DNSRouter](ctx)
		if router != nil {
			var resolveTransport adapter.DNSTransport
			if options.DomainResolver != "" {
				transport, loaded := service.FromContext[adapter.DNSTransportManager](ctx).Transport(options.DomainResolver)
				if !loaded {
					return nil, E.New("DNS server not found: " + options.DomainResolver)
				}
				resolveTransport = transport
			}
			dialer = NewResolveDialer(
				router,
				dialer,
				options.Detour == "" && !options.TCPFastOpen,
				resolveTransport,
				C.DomainStrategy(options.DomainStrategy),
				time.Duration(options.FallbackDelay))
		}
	}
	return dialer, nil
}

func NewDirect(ctx context.Context, options option.DialerOptions) (ParallelInterfaceDialer, error) {
	if options.Detour != "" {
		return nil, E.New("`detour` is not supported in direct context")
	}
	if options.IsWireGuardListener {
		return NewDefault(ctx, options)
	}
	dialer, err := NewDefault(ctx, options)
	if err != nil {
		return nil, err
	}
	var resolveTransport adapter.DNSTransport
	if options.DomainResolver != "" {
		transport, loaded := service.FromContext[adapter.DNSTransportManager](ctx).Transport(options.DomainResolver)
		if !loaded {
			return nil, E.New("DNS server not found: " + options.DomainResolver)
		}
		resolveTransport = transport
	}
	return NewResolveParallelInterfaceDialer(
		service.FromContext[adapter.DNSRouter](ctx),
		dialer,
		true,
		resolveTransport,
		C.DomainStrategy(options.DomainStrategy),
		time.Duration(options.FallbackDelay),
	), nil
}

type ParallelInterfaceDialer interface {
	N.Dialer
	DialParallelInterface(ctx context.Context, network string, destination M.Socksaddr, strategy *C.NetworkStrategy, interfaceType []C.InterfaceType, fallbackInterfaceType []C.InterfaceType, fallbackDelay time.Duration) (net.Conn, error)
	ListenSerialInterfacePacket(ctx context.Context, destination M.Socksaddr, strategy *C.NetworkStrategy, interfaceType []C.InterfaceType, fallbackInterfaceType []C.InterfaceType, fallbackDelay time.Duration) (net.PacketConn, error)
}

type ParallelNetworkDialer interface {
	DialParallelNetwork(ctx context.Context, network string, destination M.Socksaddr, destinationAddresses []netip.Addr, strategy *C.NetworkStrategy, interfaceType []C.InterfaceType, fallbackInterfaceType []C.InterfaceType, fallbackDelay time.Duration) (net.Conn, error)
	ListenSerialNetworkPacket(ctx context.Context, destination M.Socksaddr, destinationAddresses []netip.Addr, strategy *C.NetworkStrategy, interfaceType []C.InterfaceType, fallbackInterfaceType []C.InterfaceType, fallbackDelay time.Duration) (net.PacketConn, netip.Addr, error)
}
