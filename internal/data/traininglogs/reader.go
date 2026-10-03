package traininglogs

import (
    "context"
    "io"
    "strings"
    "time"
    "unicode/utf8"

    "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Reader struct { client coreclient.CoreV1Interface }

func New(client coreclient.CoreV1Interface) *Reader { return &Reader{client: client} }

func (reader *Reader) ReadTrainingLogs(ctx context.Context, source biz.TrainingLogSource, options biz.TrainingLogOptions) (biz.TrainingLogResult, error) {
    unavailable := func() (biz.TrainingLogResult, error) { return biz.TrainingLogResult{}, biz.ErrTrainingLogsUnavailable }
    if ctx == nil || reader == nil || reader.client == nil || source.Namespace == "" || source.NamespaceUID == "" ||
        source.PodName == "" || source.PodUID == "" || source.OwnerUID == "" || source.OwnerName == "" ||
        source.OwnerKind != "Job" || source.OwnerAPIVersion != "batch/v1" || source.ContainerName != "node" ||
        options.TailLines == 0 || options.TailLines > 1000 || options.MaxBytes == 0 || options.MaxBytes > 65536 {
        return unavailable()
    }
    bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    namespace, err := reader.client.Namespaces().Get(bounded, source.Namespace, metav1.GetOptions{})
    if err != nil || string(namespace.UID) != source.NamespaceUID { return unavailable() }
    pods := reader.client.Pods(source.Namespace)
    pod, err := pods.Get(bounded, source.PodName, metav1.GetOptions{})
    if err != nil || !matchesPod(pod, source) { return unavailable() }
    lines, bytes := int64(options.TailLines)+1, int64(options.MaxBytes)+1
    stream, err := pods.GetLogs(source.PodName, &corev1.PodLogOptions{Container:source.ContainerName, TailLines:&lines, LimitBytes:&bytes, Timestamps:true}).Stream(bounded)
    if err != nil { return unavailable() }
    data, readErr := io.ReadAll(io.LimitReader(stream, bytes))
    closeErr := stream.Close()
    if readErr != nil || closeErr != nil || bounded.Err() != nil { return unavailable() }
    // The log endpoint is name-based. Recheck after streaming so recreation or
    // container restart cannot silently substitute a different segment.
    current, err := pods.Get(bounded, source.PodName, metav1.GetOptions{})
    if err != nil || !matchesPod(current, source) { return unavailable() }
    result := biz.TrainingLogResult{ObservedAt:time.Now().UTC(), Truncated:len(data)>int(options.MaxBytes)}
    if result.Truncated {
        data = data[:options.MaxBytes]
        for cut:=0; cut<utf8.UTFMax-1 && len(data)>0 && !utf8.Valid(data); cut++ { data = data[:len(data)-1] }
    }
    if !utf8.Valid(data) || len(data)==0 { return unavailable() }
    rawLines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
    for index, line := range rawLines {
        stamp, text, found := strings.Cut(line, " ")
        when, err := time.Parse(time.RFC3339Nano, stamp)
        if !found || err != nil || when.IsZero() || when.Year()<1 || when.Year()>9999 {
            // A byte limit may cut the next timestamp; keep earlier readable
            // lines and make that missing tail explicit through Truncated.
            if result.Truncated && index==len(rawLines)-1 && !strings.HasSuffix(string(data),"\n") { break }
            return unavailable()
        }
        result.Lines = append(result.Lines, biz.TrainingLogLine{Timestamp:when, Text:text})
    }
    if len(result.Lines)==0 { return unavailable() }
    if len(result.Lines)>int(options.TailLines) {
        result.Lines = result.Lines[len(result.Lines)-int(options.TailLines):]
        result.Truncated = true
    }
    return result, nil
}

func matchesPod(pod *corev1.Pod, source biz.TrainingLogSource) bool {
    if pod == nil || string(pod.UID)!=source.PodUID || pod.Name!=source.PodName || pod.Namespace!=source.Namespace { return false }
    controllers := 0
    for _, owner := range pod.OwnerReferences {
        if owner.Controller==nil || !*owner.Controller { continue }
        controllers++
        if string(owner.UID)!=source.OwnerUID || owner.Name!=source.OwnerName || owner.Kind!=source.OwnerKind || owner.APIVersion!=source.OwnerAPIVersion { return false }
    }
    containers, statuses := 0, 0
    for _, container := range pod.Spec.Containers { if container.Name==source.ContainerName { containers++ } }
    for _, status := range pod.Status.ContainerStatuses {
        if status.Name==source.ContainerName { statuses++; if status.RestartCount!=0 { return false } }
    }
    return controllers==1 && containers==1 && statuses==1
}
