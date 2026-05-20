package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultConsoleGRPCDialTimeout    = 3 * time.Second
	defaultConsoleGRPCRequestTimeout = 3 * time.Second
)

// OfficialCreditChecker reads official AI credit balance from console.
type OfficialCreditChecker interface {
	GetOfficialBalance(ctx context.Context, organizationID uuid.UUID) (int64, error)
}

type consoleOfficialCreditChecker struct {
	grpcAddr       string
	dialTimeout    time.Duration
	requestTimeout time.Duration
}

// NewConsoleOfficialCreditChecker creates a checker for console official AI credits.
func NewConsoleOfficialCreditChecker() OfficialCreditChecker {
	return &consoleOfficialCreditChecker{
		grpcAddr:       resolveConsoleGRPCAddr(),
		dialTimeout:    defaultConsoleGRPCDialTimeout,
		requestTimeout: defaultConsoleGRPCRequestTimeout,
	}
}

func (c *consoleOfficialCreditChecker) GetOfficialBalance(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	if strings.TrimSpace(c.grpcAddr) == "" {
		return 0, fmt.Errorf("%w: console grpc address is empty", ErrOfficialCreditUnavailable)
	}

	dialCtx, cancelDial := context.WithTimeout(ctx, c.dialTimeout)
	defer cancelDial()

	conn, err := grpc.DialContext(
		dialCtx,
		c.grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return 0, fmt.Errorf("%w: dial console grpc failed: %v", ErrOfficialCreditUnavailable, err)
	}
	defer conn.Close()

	reqCtx, cancelReq := context.WithTimeout(ctx, c.requestTimeout)
	defer cancelReq()

	client := pb.NewBillingServiceClient(conn)
	resp, err := client.CheckCreditBalance(reqCtx, &pb.CheckCreditBalanceRequest{
		OrganizationId:   organizationID.String(),
		EstimatedCredits: 0,
	})
	if err != nil {
		return 0, fmt.Errorf("%w: check credit balance rpc failed: %v", ErrOfficialCreditUnavailable, err)
	}

	if resp == nil || !resp.Success {
		msg := ""
		if resp != nil {
			msg = resp.ErrorMessage
		}
		return 0, fmt.Errorf("%w: check credit balance rpc unsuccessful: %s", ErrOfficialCreditUnavailable, msg)
	}

	return resp.Balance, nil
}

func resolveConsoleGRPCAddr() string {
	return strings.TrimSpace(config.Current().Console.GRPCAddr)
}
