package submittest_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/runtimeproof"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/stepidentity"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/workspace"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/commandtls"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/server"
	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"google.golang.org/grpc"
	grpccredentials "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// KFP/Kubernetes/S3 control responses are external-system substitutes. All
// ModelDev code, RPC authorization, PostgreSQL transactions, transfers and MLP
// calculations run for real. This is a reproducible module flow, not L1-L4.
func TestMainFlowCompleteActualMLPToVerifiedPublicationAndClosed(t *testing.T){
	f:=newCompleteFixture(t)
	open:=postgres.Prepare(t);pool:=open();admissions:=execution.New(pool);dispatch:=submission.New(pool);facts:=lifecycle.New(pool)
	ctx,cancel:=context.WithTimeout(context.Background(),150*time.Second);defer cancel()
	acceptThroughCommandRPC(t,ctx,admissions,f.request.Admission)
	provider:=tokenProviderFunc(func(_ context.Context,tenant string,env cpup01.EnvironmentBindingSnapshot)(string,error){if tenant!=f.request.Admission.TenantID||env!=f.request.Admission.Snapshot.Environment{return "",fmt.Errorf("unexpected frozen KFP binding")};return "synthetic-kfp-owner",nil})
	roots:=x509.NewCertPool();roots.AddCert(f.kfp.Certificate());runs,err:=kfp.New(kfp.Config{ConnectionRef:f.request.Admission.Snapshot.Environment.KFPConnectionRef,Endpoint:f.kfp.URL,RootCAs:roots,Timeout:5*time.Second},provider);if err!=nil{t.Fatal(err)}
	submitter,err:=biz.NewPipelineSubmitter(dispatch,runs,3*time.Second);if err!=nil{t.Fatal(err)}
	if _,err:=submitter.Submit(ctx,f.request);err!=nil{t.Fatal(err)}
	kube,err:=dynamic.NewForConfig(&rest.Config{Host:f.kube.URL,BearerToken:"synthetic-kube-owner",Timeout:10*time.Second,TLSClientConfig:rest.TLSClientConfig{CAData:pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:f.kube.Certificate().Raw})}});if err!=nil{t.Fatal(err)}
	identity,err:=stepidentity.New(kube,"ani-modeldev-managed-step");if err!=nil{t.Fatal(err)}
	store:=s3.New(s3.Options{Region:"us-east-1",Credentials:credentials.NewStaticCredentialsProvider("synthetic-key","synthetic-secret",""),BaseEndpoint:aws.String(f.storage.URL),UsePathStyle:true,HTTPClient:f.storage.Client(),RetryMaxAttempts:1,RequestChecksumCalculation:aws.RequestChecksumCalculationWhenRequired,ResponseChecksumValidation:aws.ResponseChecksumValidationWhenRequired})
	proof:=runtimeproof.New(kube,runs,objectstore.NewVerifier(store,f.request.Admission.Snapshot.PublicationScope.StorageConnectionID,64<<20),dispatch)
	steps,err:=biz.NewManagedSteps(dispatch,admissions,identity,runs);if err!=nil{t.Fatal(err)}
	managed,err:=biz.NewManagedRuntime(steps,facts,trainer.New(kube),proof,proof);if err!=nil{t.Fatal(err)}
	client,stop:=startMainFlowStepHandler(t,service.NewRuntimeStep(steps,managed))
	call:=func(step string)context.Context{return metadata.NewOutgoingContext(ctx,metadata.Pairs("authorization","Bearer synthetic-bound-"+step,"x-ani-tenant-id",f.request.Admission.TenantID))}
	if _,err:=client.BeginExecution(call("prepare"),&modeldevv1.BeginExecutionRequest{Context:f.stepContext("prepare")});err!=nil{t.Fatal(err)}
	t.Log("MAIN_FLOW: Governance mTLS command -> durable inbox -> one KFP CreateRun -> authenticated Begin")
	configuration,err:=client.GetExecutionConfiguration(call("prepare"),&modeldevv1.GetExecutionConfigurationRequest{Context:f.stepContext("prepare")});if err!=nil{t.Fatalf("MAIN_FLOW_RUNTIME_NOT_IMPLEMENTED: execution configuration failed: %v",err)}
	original,err:=admissions.Get(ctx,f.request.Admission.TenantID,f.request.Admission.ExecutionID);if err!=nil{t.Fatal(err)}
	prepared,err:=workspace.PrepareInput(ctx,store,original,f.workspace,f.root);if err!=nil{t.Fatal(err)}
	prepareReport:=&modeldevv1.ReportStepResultRequest{Context:f.stepContext("prepare"),ReportId:"11111111-1111-4111-8111-111111111111",Result:&modeldevv1.ReportStepResultRequest_Prepared{Prepared:&modeldevv1.PreparedInputCandidate{Input:configuration.Snapshot.Input,Workspace:f.workspaceProto(),PreparedManifest:&trainingv1.ManifestRef{RelativePath:"prepared-manifest.json",SizeBytes:prepared.PreparedManifestBytes,Sha256:prepared.PreparedManifestSHA256}}}}
	if _,err:=client.ReportStepResult(call("prepare"),prepareReport);err!=nil{t.Fatal(err)}
	if repeated,err:=client.ReportStepResult(call("prepare"),prepareReport);err!=nil||!repeated.GetReplayed(){t.Fatalf("prepare replay failed: %v",err)}
	t.Log("MAIN_FLOW: fixed S3 CSV actual bytes verified and prepared workspace committed")
	for _,subpath:=range []string{f.workspace.TrainingSubpath,f.workspace.PublicationSubpath}{if err:=os.MkdirAll(filepath.Join(f.root,subpath),0700);err!=nil{t.Fatal(err)}}
	ensured,err:=client.EnsureTraining(call("train-wait"),&modeldevv1.EnsureTrainingRequest{Context:f.stepContext("train-wait")});if err!=nil||ensured.GetStatus().GetTrainjob().GetUid()!=completeTrainUID{t.Fatalf("training creation failed: %v",err)}
	if again,err:=client.EnsureTraining(call("train-wait"),&modeldevv1.EnsureTrainingRequest{Context:f.stepContext("train-wait")});err!=nil||again.GetStatus().GetTrainjob().GetUid()!=completeTrainUID{t.Fatalf("duplicate Ensure changed training: %v",err)}
	select{case<-f.trainingDone:case<-ctx.Done():t.Fatal("actual training timed out")}
	f.mu.Lock();trainingErr,trainingLog:=f.trainingErr,string(f.trainingLog);f.mu.Unlock();if trainingErr!=nil{t.Fatalf("actual MLP failed: %v\n%s",trainingErr,trainingLog)}
	status,err:=client.GetTrainingStatus(call("train-wait"),&modeldevv1.GetTrainingStatusRequest{Context:f.stepContext("train-wait")});if err!=nil||status.GetStatus().GetStates().GetComputeState()!=modeldevv1.ComputeState_COMPUTE_STATE_SUCCEEDED{t.Fatalf("actual training exit did not propagate: %v",err)}
	summary,err:=os.ReadFile(filepath.Join(f.root,f.workspace.TrainingSubpath,"summary.json"));if err!=nil{t.Fatal(err)};var metrics map[string]any;if json.Unmarshal(summary,&metrics)!=nil||metrics["steps"]!=float64(48){t.Fatalf("MLP did not execute 48 optimizer steps: %s",summary)}
	t.Log("MAIN_FLOW: exactly one TrainJob -> real CPU MLP completed 48 optimizer steps -> Pod exit observed")
	output,err:=workspace.Collect(ctx,filepath.Join(f.root,f.workspace.TrainingSubpath),original);if err!=nil{t.Fatal(err)}
	publication,err:=workspace.UploadOutput(ctx,store,original,filepath.Join(f.root,f.workspace.TrainingSubpath),output);if err!=nil{t.Fatal(err)}
	publishReport:=&modeldevv1.ReportStepResultRequest{Context:f.stepContext("close"),ReportId:"22222222-2222-4222-8222-222222222222",Result:&modeldevv1.ReportStepResultRequest_Publication{Publication:publicationProto(publication)}}
	if _,err:=client.ReportStepResult(call("close"),publishReport);err==nil{t.Fatal("running uploader incorrectly granted PUBLISHED")}
	f.finishPublisher()
	published,err:=client.ReportStepResult(call("close"),publishReport);if err!=nil||published.GetPublication().GetPublicationId()==""||published.GetStates().GetDeliveryState()!=modeldevv1.DeliveryState_DELIVERY_STATE_PUBLISHED{t.Fatalf("remote verified publication failed: %v",err)}
	if replay,err:=client.ReportStepResult(call("close"),publishReport);err!=nil||!replay.GetReplayed()||replay.GetPublication().GetPublicationId()!=published.Publication.PublicationId{t.Fatalf("publication retry changed receipt: %v",err)}
	t.Log("MAIN_FLOW: actual files + manifest + tar uploaded; completed uploader and remote bytes verified before publication")
	// Simulated KFP acknowledges completed tasks only after their real component
	// work above returns. The close task does not mount/write training outputs.
	for _,step:=range []string{"prepare","train-wait","collect"}{f.finishStep(step)}
	closed,err:=client.RequestExecutionClose(call("close"),&modeldevv1.RequestExecutionCloseRequest{Context:f.stepContext("close"),Reason:modeldevv1.CloseReason_CLOSE_REASON_NATURAL_TERMINAL});if err!=nil||closed.GetCloseState()!=modeldevv1.CloseState_CLOSE_STATE_CLOSED{t.Fatalf("actual writer proof did not close: %v",err)}
	stop();pool.Close();pool=open();admissions=execution.New(pool);dispatch=submission.New(pool);facts=lifecycle.New(pool)
	stored,err:=facts.GetRuntime(ctx,original.TenantID,original.ExecutionID);if err!=nil||stored.ClosedAt==nil||stored.Publication==nil||stored.TrainingHandle==nil{t.Fatalf("reconnected owner lost runtime: %v",err)}
	if _,err:=biz.NewPipelineSubmitter(dispatch,runs,3*time.Second);err!=nil{t.Fatal(err)}
	f.mu.Lock();creates,runCreates:=f.creates,f.runCreates;f.mu.Unlock();if creates!=1||runCreates!=1{t.Fatalf("main flow repeated creation: Run=%d TrainJob=%d",runCreates,creates)}
	// This verifier directory has no training mount. It consumes only objects
	// referenced by the durable publication; this is not BFF/L4 authorization.
	independent:=t.TempDir();for _,file:=range stored.Publication.Files{object,err:=store.GetObject(ctx,&s3.GetObjectInput{Bucket:aws.String(file.Object.Bucket),Key:aws.String(file.Object.Key),VersionId:file.Object.VersionID});if err!=nil{t.Fatal(err)};data,err:=io.ReadAll(object.Body);object.Body.Close();if err!=nil||int64(len(data))!=file.File.SizeBytes||completeHash(data)!=file.File.SHA256{t.Fatal("independent download byte verification failed")};if err:=os.WriteFile(filepath.Join(independent,file.File.RelativePath),data,0600);err!=nil{t.Fatal(err)}}
	reload:=exec.CommandContext(ctx,"podman","run","--rm","--pull=never","--network","none","--http-proxy=false","--cpus","2","--memory","2g","--memory-swap","2g","--pids-limit","256","--cap-drop","all","--security-opt","no-new-privileges","--read-only","--userns","keep-id:uid=10001,gid=10001","--volume",f.source+":/source:ro,Z","--volume",independent+":/download:ro,Z","--entrypoint","/opt/venv/bin/python",f.image,"-I","/source/tests/reload_checkpoint.py","/download")
	forward,err:=reload.CombinedOutput();if err!=nil{t.Fatalf("independent weights-only reload failed: %v: %s",err,forward)}
	t.Logf("MAIN_FLOW: CLOSED persisted; retained original workspace; independently downloaded checkpoint forward=%s",forward)
	if evidence:=os.Getenv("CPU_P01_MAINFLOW_OUTPUT");evidence!=""{if err:=os.MkdirAll(evidence,0700);err!=nil{t.Fatal(err)};for name,contents:=range map[string][]byte{"training.log":[]byte(trainingLog),"summary.json":summary,"output-manifest.json":output.Manifest,"forward.json":forward}{if err:=os.WriteFile(filepath.Join(evidence,name),contents,0600);err!=nil{t.Fatal(err)}}}
}

func(f *completeFixture) stepContext(step string)*modeldevv1.StepContext{return &modeldevv1.StepContext{Identity:&trainingv1.ExecutionIdentity{OperationId:f.request.Admission.OperationID,ExecutionId:f.request.Admission.ExecutionID,ExecutionSpecHash:f.request.Admission.SpecHash},Association:&modeldevv1.RunAssociation{KfpRunId:completeRunID,NamespaceName:f.workspace.NamespaceName,NamespaceUid:f.workspace.NamespaceUID,WorkflowName:"main-flow",WorkflowUid:completeWorkflowUID,PodName:"main-"+step,PodUid:completeStepUID(step)},Step:map[string]modeldevv1.PipelineStep{"prepare":modeldevv1.PipelineStep_PIPELINE_STEP_PREPARE,"train-wait":modeldevv1.PipelineStep_PIPELINE_STEP_TRAIN_WAIT,"close":modeldevv1.PipelineStep_PIPELINE_STEP_CLOSE}[step]}}
func(f *completeFixture) workspaceProto()*trainingv1.WorkspaceRef{b:=f.workspace;return &trainingv1.WorkspaceRef{Mode:trainingv1.WorkspaceMode_WORKSPACE_MODE_EXECUTION_PVC,NamespaceName:b.NamespaceName,NamespaceUid:b.NamespaceUID,PvcName:b.PVCName,PvcUid:b.PVCUID,InputSubpath:b.InputSubpath,TrainingSubpath:b.TrainingSubpath,ReportsSubpath:b.ReportsSubpath,PublicationSubpath:b.PublicationSubpath}}
func fixedObjectProto(o cpup01.FixedObjectRef)*trainingv1.FixedObjectRef{return &trainingv1.FixedObjectRef{StorageConnectionId:o.StorageConnectionID,Bucket:o.Bucket,Key:o.Key,Immutability:&trainingv1.FixedObjectRef_VersionId{VersionId:*o.VersionID},SizeBytes:o.SizeBytes,Sha256:o.SHA256}}
func publicationProto(p biz.RuntimePublication)*modeldevv1.PublicationCandidate{
	r:=&modeldevv1.PublicationCandidate{LogicalPublicationKey:p.LogicalKey,ManifestObject:fixedObjectProto(p.Manifest),UploadTaskId:"publish-task",UploadPodUid:completeStepUID("publish")};if p.Bundle!=nil{r.BundleObject=fixedObjectProto(*p.Bundle)};for _,f:=range p.Files{r.Files=append(r.Files,&modeldevv1.PublishedFileCandidate{File:&trainingv1.FileEntry{RelativePath:f.File.RelativePath,Role:trainingv1.FileRole(trainingv1.FileRole_value["FILE_ROLE_"+f.File.Role]),SizeBytes:f.File.SizeBytes,Sha256:f.File.SHA256},Object:fixedObjectProto(f.Object)})};return r
}
func(f *completeFixture) finishStep(step string){f.mu.Lock();defer f.mu.Unlock();f.objects["/api/v1/namespaces/"+f.workspace.NamespaceName+"/pods/main-"+step]["status"]=map[string]any{"phase":"Succeeded","containerStatuses":[]any{map[string]any{"name":"main","restartCount":0,"state":map[string]any{"terminated":map[string]any{"exitCode":0,"finishedAt":time.Now().UTC().Format(time.RFC3339)}}}}}}

func acceptThroughCommandRPC(t *testing.T,ctx context.Context,repository biz.ExecutionRepository,admission biz.Admission){
	t.Helper();certs:=commandtls.New(t);listener,err:=server.NewGovernanceCommandServer(&conf.Server_GRPC{Network:"tcp",Addr:"127.0.0.1:0",Timeout:durationpb.New(5*time.Second)},server.CommandTLS{Certificate:certs.Server,ClientCAs:certs.Roots,GovernanceDNSName:commandtls.GovernanceDNSName},service.NewCommand(repository),nil);if err!=nil{t.Fatal(err)};endpoint,err:=listener.Endpoint();if err!=nil{t.Fatal(err)};done:=make(chan error,1);go func(){done<-listener.Start(context.Background())}();defer func(){if err:=listener.Stop(ctx);err!=nil{t.Error(err)};<-done}()
	connection,err:=grpc.NewClient(endpoint.Host,grpc.WithTransportCredentials(grpccredentials.NewTLS(&tls.Config{MinVersion:tls.VersionTLS13,RootCAs:certs.Roots,ServerName:commandtls.ServerDNSName,Certificates:[]tls.Certificate{certs.Governance}})));if err!=nil{t.Fatal(err)};defer connection.Close()
	snapshot,err:=contractpb.EncodeSnapshot(admission.Snapshot);if err!=nil{t.Fatal(err)};intent,err:=contractpb.EncodeIntent(admission.Intent);if err!=nil{t.Fatal(err)}
	request:=&modeldevv1.AcceptExecutionRequest{Identity:&trainingv1.ExecutionIdentity{OperationId:admission.OperationID,ExecutionId:admission.ExecutionID,ExecutionSpecHash:admission.SpecHash},ResourceTenantId:admission.TenantID,AdmittedActorId:admission.Actor,Intent:intent,IntentHash:admission.IntentHash,Snapshot:snapshot,AcceptedAt:timestamppb.New(admission.AcceptedAt)}
	call:=metadata.NewOutgoingContext(ctx,metadata.Pairs("x-ani-tenant-id",admission.TenantID,"x-ani-actor",admission.Actor,"x-ani-request-id","99999999-1111-4222-8333-444444444444"));if _,err:=modeldevv1.NewModelDevCommandServiceClient(connection).AcceptExecution(call,request);err!=nil{t.Fatal(err)}
}
