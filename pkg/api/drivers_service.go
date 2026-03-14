package api

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/core"
	driverspkg "github.com/relaymesh/relaymesh/pkg/drivers"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// DriversService handles CRUD for driver configs.
type DriversService struct {
	Store  storage.DriverStore
	Cache  *driverspkg.Cache
	Logger *log.Logger
}

func (s *DriversService) ListDrivers(
	ctx context.Context,
	req *connect.Request[cloudv1.ListDriversRequest],
) (*connect.Response[cloudv1.ListDriversResponse], error) {
	_ = req
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	records, err := s.Store.ListDrivers(ctx)
	if err != nil {
		logError(s.Logger, "list drivers failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list drivers failed"))
	}
	resp := &cloudv1.ListDriversResponse{
		Drivers: toProtoDriverRecords(records),
	}
	return connect.NewResponse(resp), nil
}

func (s *DriversService) GetDriver(
	ctx context.Context,
	req *connect.Request[cloudv1.GetDriverRequest],
) (*connect.Response[cloudv1.GetDriverResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	name := strings.TrimSpace(req.Msg.GetName())
	record, err := s.Store.GetDriver(ctx, name)
	if err != nil {
		logError(s.Logger, "get driver failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("get driver failed"))
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("driver not found"))
	}
	resp := &cloudv1.GetDriverResponse{
		Driver: toProtoDriverRecord(record),
	}
	return connect.NewResponse(resp), nil
}

func (s *DriversService) UpsertDriver(
	ctx context.Context,
	req *connect.Request[cloudv1.UpsertDriverRequest],
) (*connect.Response[cloudv1.UpsertDriverResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	driver := req.Msg.GetDriver()
	name := strings.TrimSpace(driver.GetName())
	configJSON := strings.TrimSpace(driver.GetConfigJson())

	// Validate driver config before persisting — fail fast on bad config.
	if err := validateDriverConfig(name, configJSON); err != nil {
		logError(s.Logger, "driver config validation failed", err)
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	record, err := s.Store.UpsertDriver(ctx, storage.DriverRecord{
		ID:         strings.TrimSpace(driver.GetId()),
		Name:       name,
		ConfigJSON: configJSON,
		Enabled:    driver.GetEnabled(),
	})
	if err != nil {
		logError(s.Logger, "upsert driver failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("upsert driver failed"))
	}
	if s.Cache != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := s.Cache.Refresh(refreshCtx); err != nil {
			logError(s.Logger, "driver cache refresh failed", err)
		}
		cancel()
	}
	resp := &cloudv1.UpsertDriverResponse{
		Driver: toProtoDriverRecord(record),
	}
	return connect.NewResponse(resp), nil
}

// validateDriverConfig validates a driver config before persisting.
func validateDriverConfig(name, configJSON string) error {
	if name == "" {
		return errors.New("driver name is required")
	}
	cfg, err := driverspkg.ConfigFromDriver(name, configJSON)
	if err != nil {
		return err
	}
	return core.ValidatePublisherConfig(cfg)
}

func (s *DriversService) DeleteDriver(
	ctx context.Context,
	req *connect.Request[cloudv1.DeleteDriverRequest],
) (*connect.Response[cloudv1.DeleteDriverResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	name := strings.TrimSpace(req.Msg.GetName())
	if err := s.Store.DeleteDriver(ctx, name); err != nil {
		logError(s.Logger, "delete driver failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("delete driver failed"))
	}
	if s.Cache != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := s.Cache.Refresh(refreshCtx); err != nil {
			logError(s.Logger, "driver cache refresh failed", err)
		}
		cancel()
	}
	return connect.NewResponse(&cloudv1.DeleteDriverResponse{}), nil
}
