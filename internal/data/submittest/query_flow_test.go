//go:build cpu_mainflow

package submittest_test

import (
 "context"
 "crypto/tls"
 "testing"
 "time"

 "github.com/aws/aws-sdk-go-v2/service/s3"
 "github.com/jackc/pgx/v5/pgxpool"
 modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
 conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/server"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
 "google.golang.org/grpc"
 "google.golang.org/grpc/codes"
 "google.golang.org/grpc/credentials"
 "google.golang.org/grpc/metadata"
 "google.golang.org/grpc/status"
 "google.golang.org/protobuf/types/known/durationpb"
)

func startMainFlowQuery(t *testing.T, pool *pgxpool.Pool, store *s3.Client, admission biz.Admission) modeldevv1.ModelDevQueryServiceClient {
 t.Helper()
 certs := commandtls.New(t)
 repository := execution.New(pool)
 handler := service.NewQuery(repository, objectstore.NewDownloadSigner(store, admission.Snapshot.PublicationScope.StorageConnectionID))
 listener, err := server.NewGovernanceQueryServer(&conf.Server_GRPC{Network:"tcp", Addr:"127.0.0.1:0", Timeout:durationpb.New(5*time.Second)}, server.CommandTLS{Certificate:certs.Server, ClientCAs:certs.Roots, GovernanceDNSName:commandtls.GovernanceDNSName}, service.NewCommand(repository), nil, handler)
 if err != nil { t.Fatal(err) }
 endpoint, err := listener.Endpoint()
 if err != nil { t.Fatal(err) }
 done := make(chan error, 1)
 go func(){ done <- listener.Start(context.Background()) }()
 connection, err := grpc.NewClient(endpoint.Host, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion:tls.VersionTLS13, RootCAs:certs.Roots, ServerName:commandtls.ServerDNSName, Certificates:[]tls.Certificate{certs.Governance}})))
 if err != nil { t.Fatal(err) }
 t.Cleanup(func(){
  _ = connection.Close()
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second); defer cancel()
  if err := listener.Stop(ctx); err != nil { t.Error(err) }; <-done
 })
 return modeldevv1.NewModelDevQueryServiceClient(connection)
}

func queryCall(ctx context.Context, admission biz.Admission, method string) context.Context {
 return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id",admission.TenantID,"x-ani-actor","governance:user:9001","x-ani-request-id","99999999-1111-4222-8333-444444444444","x-ani-authorized-method",method,"x-ani-data-scope","tenant-all"))
}

func assertQueryAccess(t *testing.T, ctx context.Context, client modeldevv1.ModelDevQueryServiceClient, admission biz.Admission) {
 t.Helper()
 method := modeldevv1.ModelDevQueryService_GetExecution_FullMethodName
 request := &modeldevv1.GetExecutionRequest{ExecutionId:admission.ExecutionID}
 got, err := client.GetExecution(queryCall(ctx,admission,method),request)
 if err != nil || got.GetExecution().GetIdentity().GetExecutionId() != admission.ExecutionID || got.GetExecution().GetName() != admission.Intent.Name {
  t.Fatalf("QUERY_MAIN_FLOW_NOT_IMPLEMENTED: currently authorized reader cannot read durable execution: %v", err)
 }
 // A different current authorized actor may read; historical admission actor
 // is audit data. Missing or wrong per-method delegation cannot read.
 for _, candidate := range []context.Context{ctx, queryCall(ctx,admission,modeldevv1.ModelDevQueryService_AuthorizeArtifactDownload_FullMethodName)} {
  if _, err := client.GetExecution(candidate,request); status.Code(err) != codes.Unauthenticated && status.Code(err) != codes.PermissionDenied { t.Fatalf("query delegation bypass: %v",err) }
 }
 other := admission; other.TenantID="99999999-aaaa-4bbb-8ccc-111111111111"
 if _, err := client.GetExecution(queryCall(ctx,other,method),request); status.Code(err)!=codes.NotFound { t.Fatalf("cross-tenant execution exposed: %v",err) }
 absent := &modeldevv1.AuthorizeArtifactDownloadRequest{ArtifactId:"eeeeeeee-1111-4222-8333-444444444444"}
 if _, err := client.AuthorizeArtifactDownload(queryCall(ctx,admission,modeldevv1.ModelDevQueryService_AuthorizeArtifactDownload_FullMethodName),absent); status.Code(err)!=codes.NotFound { t.Fatalf("unpublished/unknown artifact not hidden: %v",err) }
}
