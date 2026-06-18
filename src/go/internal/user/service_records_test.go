package user

import (
	"context"
	"errors"
	"testing"

	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestDeleteCounterRPC(t *testing.T) {
	svc, store, _, _ := setupTest()
	ctx := context.Background()

	t.Run("EmptyFields", func(t *testing.T) {
		_, err := svc.DeleteCounter(ctx, &pbsvc.DeleteCounterRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.DeleteCounter(ctx, &pbsvc.DeleteCounterRequest{UserId: "u1", CounterId: "c1"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})

	t.Run("Success", func(t *testing.T) {
		_, err := svc.DeleteCounter(ctx, &pbsvc.DeleteCounterRequest{UserId: "u1", CounterId: "c1"})
		assert.NoError(t, err)
	})
}

func TestPersonalRecordRPCs(t *testing.T) {
	svc, store, _, _ := setupTest()
	ctx := context.Background()

	t.Run("List_EmptyUserId", func(t *testing.T) {
		_, err := svc.ListPersonalRecords(ctx, &pbsvc.ListPersonalRecordsRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("List_StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.ListPersonalRecords(ctx, &pbsvc.ListPersonalRecordsRequest{UserId: "u1"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("List_Success", func(t *testing.T) {
		resp, err := svc.ListPersonalRecords(ctx, &pbsvc.ListPersonalRecordsRequest{UserId: "u1"})
		assert.NoError(t, err)
		assert.NotNil(t, resp.Records)
	})

	t.Run("Set_EmptyFields", func(t *testing.T) {
		_, err := svc.SetPersonalRecord(ctx, &pbsvc.SetPersonalRecordRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("Set_StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.SetPersonalRecord(ctx, &pbsvc.SetPersonalRecordRequest{UserId: "u1", RecordType: "5k", Value: 1, Unit: "s"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("Set_Success", func(t *testing.T) {
		rec, err := svc.SetPersonalRecord(ctx, &pbsvc.SetPersonalRecordRequest{UserId: "u1", RecordType: "5k", Value: 1500, Unit: "s", ActivityId: "a1"})
		assert.NoError(t, err)
		assert.Equal(t, "5k", rec.RecordType)
		assert.Equal(t, "a1", rec.ActivityId)
	})

	t.Run("Delete_EmptyFields", func(t *testing.T) {
		_, err := svc.DeletePersonalRecord(ctx, &pbsvc.DeletePersonalRecordRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("Delete_StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.DeletePersonalRecord(ctx, &pbsvc.DeletePersonalRecordRequest{UserId: "u1", RecordType: "5k"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("Delete_Success", func(t *testing.T) {
		_, err := svc.DeletePersonalRecord(ctx, &pbsvc.DeletePersonalRecordRequest{UserId: "u1", RecordType: "5k"})
		assert.NoError(t, err)
	})
}

func TestPluginDefaultsRPCs(t *testing.T) {
	svc, store, _, _ := setupTest()
	ctx := context.Background()

	t.Run("List_EmptyUserId", func(t *testing.T) {
		_, err := svc.ListPluginDefaults(ctx, &pbsvc.ListPluginDefaultsRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("List_StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.ListPluginDefaults(ctx, &pbsvc.ListPluginDefaultsRequest{UserId: "u1"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("List_Success", func(t *testing.T) {
		resp, err := svc.ListPluginDefaults(ctx, &pbsvc.ListPluginDefaultsRequest{UserId: "u1"})
		assert.NoError(t, err)
		assert.NotNil(t, resp.Defaults)
	})

	t.Run("Set_EmptyFields", func(t *testing.T) {
		_, err := svc.SetPluginDefaults(ctx, &pbsvc.SetPluginDefaultsRequest{UserId: "u1", PluginId: "p1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("Set_StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.SetPluginDefaults(ctx, &pbsvc.SetPluginDefaultsRequest{UserId: "u1", PluginId: "p1", Defaults: &structpb.Struct{}})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("Set_Success", func(t *testing.T) {
		_, err := svc.SetPluginDefaults(ctx, &pbsvc.SetPluginDefaultsRequest{UserId: "u1", PluginId: "p1", Defaults: &structpb.Struct{}})
		assert.NoError(t, err)
	})

	t.Run("Delete_EmptyFields", func(t *testing.T) {
		_, err := svc.DeletePluginDefaults(ctx, &pbsvc.DeletePluginDefaultsRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("Delete_StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.DeletePluginDefaults(ctx, &pbsvc.DeletePluginDefaultsRequest{UserId: "u1", PluginId: "p1"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("Delete_Success", func(t *testing.T) {
		_, err := svc.DeletePluginDefaults(ctx, &pbsvc.DeletePluginDefaultsRequest{UserId: "u1", PluginId: "p1"})
		assert.NoError(t, err)
	})
}

func TestSetFCMTokenRPC(t *testing.T) {
	svc, store, _, _ := setupTest()
	ctx := context.Background()

	t.Run("EmptyFields", func(t *testing.T) {
		_, err := svc.SetFCMToken(ctx, &pbsvc.SetFCMTokenRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
	t.Run("StoreError", func(t *testing.T) {
		store.err = errors.New("boom")
		_, err := svc.SetFCMToken(ctx, &pbsvc.SetFCMTokenRequest{UserId: "u1", Token: "t1"})
		assert.Equal(t, codes.Internal, status.Code(err))
		store.err = nil
	})
	t.Run("Success", func(t *testing.T) {
		_, err := svc.SetFCMToken(ctx, &pbsvc.SetFCMTokenRequest{UserId: "u1", Token: "t1"})
		assert.NoError(t, err)
	})
}
