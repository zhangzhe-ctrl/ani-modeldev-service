package tenantenvironment

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func subjectIdentity(s Subject, namespace string) (string, error) {
	switch s.Kind {
	case "User", "Group":
		if s.Name == "" || s.Namespace != "" {
			return "", biz.ErrTenantEnvironmentInvalid
		}
		return s.Kind + ":" + s.Name, nil
	case "ServiceAccount":
		if s.Namespace == "" {
			s.Namespace = namespace
		}
		if s.Name == "" || s.Namespace == "" {
			return "", biz.ErrTenantEnvironmentInvalid
		}
		return "User:system:serviceaccount:" + s.Namespace + ":" + s.Name, nil
	}
	return "", biz.ErrTenantEnvironmentInvalid
}
func canonicalDigest(v any) string       { raw, _ := json.Marshal(v); return biz.EnvironmentDigest(raw) }
func (p *Platform) policyDigest() string { return canonicalDigest(p.config.Bootstrap) }
func (p *Platform) validProof(row *biz.TenantExecutionEnvironment) error {
	proof := row.Receipts.Authorization
	if proof == nil || proof.FailureReason != "" || len(proof.DangerousEdges) > 0 || proof.OperationID != row.LastOperationID || proof.NamespaceUID != row.NamespaceUID || proof.PolicyDigest != p.policyDigest() || proof.AuthModesDigest != p.config.AuthorizationModesEvidenceSHA256 || proof.TrustedDigest != canonicalDigest(p.config.TrustedPrincipals) || !time.Now().UTC().Before(proof.ExpiresAt) || proof.RoleBindingPages < 1 || proof.ClusterRoleBindingPages < 1 {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	return nil
}
func (p *Platform) verifyBootstrap(ctx context.Context) error {
	roles := map[string]bool{"modeldev-runtime-v1": true, "modeldev-step-v1": true, "modeldev-webhook-read-v1": true, "modeldev-kfp-api-v1": true, "modeldev-kfp-access-v1": true}
	counts := map[string]int{}
	policies := map[string]bool{"modeldev-bootstrap-v1": true, "modeldev-environment-config-v1": true, "modeldev-workload-guard-v1": true}
	names := map[string]bool{}
	for _, pin := range p.config.Bootstrap {
		key := pin.Kind + "/" + pin.Name
		if names[key] || !biz.CanonicalEnvironmentUUID(pin.UID) || !validDigest(pin.SHA256) {
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		names[key] = true
		switch pin.Kind {
		case "ClusterRole":
			if !roles[pin.Name] {
				return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
			}
			delete(roles, pin.Name)
		case "ValidatingAdmissionPolicy", "ValidatingAdmissionPolicyBinding":
			policyName := strings.TrimSuffix(pin.Name, "-binding")
			if !policies[policyName] || (pin.Kind == "ValidatingAdmissionPolicyBinding" && pin.Name != policyName+"-binding") {
				return biz.EnvironmentError("INCOMPATIBLE_ADMISSION_POLICY")
			}
		case "MutatingWebhookConfiguration":
			if pin.Name != "ani-modeldev-webhook" {
				return biz.EnvironmentError("INCOMPATIBLE_ADMISSION_POLICY")
			}
		default:
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		counts[pin.Kind]++
		o, e := p.get(ctx, resource(pin.Kind), "", pin.Name)
		if e != nil || string(o.GetUID()) != pin.UID || objectDigest(o) != pin.SHA256 {
			return biz.EnvironmentError("INCOMPATIBLE_ADMISSION_POLICY")
		}
		if pin.Kind == "ValidatingAdmissionPolicy" {
			observed, ok, _ := unstructured.NestedInt64(o.Object, "status", "observedGeneration")
			_, checked, _ := unstructured.NestedMap(o.Object, "status", "typeChecking")
			warnings, _, _ := unstructured.NestedSlice(o.Object, "status", "typeChecking", "expressionWarnings")
			if !ok || observed != o.GetGeneration() || observed < 1 || !checked || len(warnings) > 0 {
				return biz.EnvironmentError("INCOMPATIBLE_ADMISSION_POLICY")
			}
		}
	}
	if len(roles) != 0 || counts["ValidatingAdmissionPolicy"] != 3 || counts["ValidatingAdmissionPolicyBinding"] != 3 || counts["MutatingWebhookConfiguration"] != 1 {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	return nil
}

type inventory struct {
	p          *Platform
	row        *biz.TenantExecutionEnvironment
	proof      biz.EnvironmentAuthorizationProof
	bytes      int
	refs       map[string]*unstructured.Unstructured
	identities map[string]string
	rules      int
}

func (p *Platform) authorization(ctx context.Context, row *biz.TenantExecutionEnvironment) (biz.EnvironmentAuthorizationProof, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	i := inventory{p: p, row: row, refs: map[string]*unstructured.Unstructured{}, identities: map[string]string{}, proof: biz.EnvironmentAuthorizationProof{OperationID: row.LastOperationID, NamespaceUID: row.NamespaceUID, PolicyDigest: p.policyDigest(), AuthModesDigest: p.config.AuthorizationModesEvidenceSHA256, TrustedDigest: canonicalDigest(p.config.TrustedPrincipals), StartedAt: time.Now().UTC(), Objects: []biz.EnvironmentAuthorizationObject{}}}
	if len(p.config.AuthorizationModes) != 2 || p.config.AuthorizationModes[0] != "Node" || p.config.AuthorizationModes[1] != "RBAC" {
		return i.proof, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	for _, kind := range []string{"RoleBinding", "ClusterRoleBinding"} {
		namespace := row.NamespaceName
		if kind == "ClusterRoleBinding" {
			namespace = ""
		}
		pages, err := i.scan(ctx, kind, namespace)
		if err != nil {
			return i.proof, err
		}
		if kind == "RoleBinding" {
			i.proof.RoleBindingPages = pages
		} else {
			i.proof.ClusterRoleBindingPages = pages
		}
	}
	if ctx.Err() != nil {
		return i.proof, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	i.proof.RuleCount = i.rules
	i.proof.CompletedAt = time.Now().UTC()
	i.proof.ExpiresAt = i.proof.CompletedAt.Add(30 * time.Second)
	sort.Slice(i.proof.Objects, func(a, b int) bool {
		x, y := i.proof.Objects[a], i.proof.Objects[b]
		return x.Kind+x.Namespace+x.Name < y.Kind+y.Namespace+y.Name
	})
	return i.proof, nil
}
func (i *inventory) account(o *unstructured.Unstructured) error {
	if o.GetName() == "" || o.GetUID() == "" || o.GetResourceVersion() == "" {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	raw, e := json.Marshal(o.Object)
	if e != nil || len(raw) > 1<<20 {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	i.bytes += len(raw)
	if i.bytes > 8<<20 {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	key := o.GetKind() + "/" + o.GetNamespace() + "/" + o.GetName()
	identity := string(o.GetUID()) + "/" + o.GetResourceVersion() + "/" + canonicalDigest(o.Object)
	if prior, ok := i.identities[key]; ok && prior != identity {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	if _, ok := i.identities[key]; !ok {
		i.identities[key] = identity
		i.proof.Objects = append(i.proof.Objects, biz.EnvironmentAuthorizationObject{Kind: o.GetKind(), Namespace: o.GetNamespace(), Name: o.GetName(), UID: string(o.GetUID()), ResourceVersion: o.GetResourceVersion(), Digest: canonicalDigest(o.Object)})
	}
	return nil
}
func (i *inventory) scan(ctx context.Context, kind, namespace string) (int, error) {
	token := ""
	seen := map[string]bool{}
	rv := ""
	for page := 1; page <= 10; page++ {
		call, cancel := context.WithTimeout(ctx, 2*time.Second)
		list, e := i.p.kube.Resource(resource(kind)).Namespace(namespace).List(call, metav1.ListOptions{Limit: 100, Continue: token})
		cancel()
		if e != nil || list == nil || len(list.Items) > 100 {
			return page, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		raw, _ := json.Marshal(list.Object)
		if len(raw) > 1<<20 {
			return page, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		if rv != "" && rv != list.GetResourceVersion() {
			return page, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		rv = list.GetResourceVersion()
		for index := range list.Items {
			o := &list.Items[index]
			if o.GetNamespace() != namespace || o.GetKind() != kind {
				return page, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
			}
			if e = i.account(o); e != nil {
				return page, e
			}
			if e = i.binding(ctx, o); e != nil {
				return page, e
			}
		}
		token = list.GetContinue()
		if token == "" {
			return page, nil
		}
		if seen[token] {
			return page, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		seen[token] = true
	}
	return 10, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
}
func (i *inventory) binding(ctx context.Context, b *unstructured.Unstructured) error {
	kind, _, _ := unstructured.NestedString(b.Object, "roleRef", "kind")
	name, _, _ := unstructured.NestedString(b.Object, "roleRef", "name")
	group, _, _ := unstructured.NestedString(b.Object, "roleRef", "apiGroup")
	if group != "rbac.authorization.k8s.io" || name == "" || (kind != "Role" && kind != "ClusterRole") || (b.GetKind() == "ClusterRoleBinding" && kind != "ClusterRole") {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	namespace := b.GetNamespace()
	if kind == "ClusterRole" {
		namespace = ""
	}
	key := kind + "/" + namespace + "/" + name
	role := i.refs[key]
	if role == nil {
		if len(i.refs) >= 128 {
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		call, cancel := context.WithTimeout(ctx, 2*time.Second)
		var e error
		role, e = i.p.kube.Resource(resource(kind)).Namespace(namespace).Get(call, name, metav1.GetOptions{})
		cancel()
		if e != nil || role.GetKind() != kind || role.GetNamespace() != namespace || role.GetName() != name {
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		if e = i.account(role); e != nil {
			return e
		}
		i.refs[key] = role
	}
	rules, ok, e := unstructured.NestedSlice(role.Object, "rules")
	if e != nil || !ok {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	i.rules += len(rules)
	if i.rules > 8192 {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	subjects, ok, e := unstructured.NestedSlice(b.Object, "subjects")
	if e != nil || !ok || len(subjects) == 0 {
		return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
	}
	trusted := map[string]bool{}
	for _, s := range i.p.config.TrustedPrincipals {
		id, e := subjectIdentity(s, "")
		if e != nil {
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		trusted[id] = true
	}
	for _, raw := range subjects {
		m, ok := raw.(map[string]any)
		if !ok {
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		s := Subject{}
		s.Kind, _ = m["kind"].(string)
		s.Name, _ = m["name"].(string)
		s.Namespace, _ = m["namespace"].(string)
		id, e := subjectIdentity(s, b.GetNamespace())
		if e != nil {
			return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
		}
		if trusted[id] {
			continue
		}
		stepID := "User:system:serviceaccount:" + i.row.NamespaceName + ":modeldev-step"
		if id == stepID && b.GetKind() == "RoleBinding" && b.GetName() == "modeldev-step" && kind == "ClusterRole" && name == "modeldev-step-v1" && len(subjects) == 1 && b.GetLabels()["modeldev.ani.io/environment-id"] == i.row.ID {
			approved := false
			for _, pin := range i.p.config.Bootstrap {
				if pin.Kind == "ClusterRole" && pin.Name == name && pin.UID == string(role.GetUID()) && pin.SHA256 == objectDigest(role) {
					approved = true
				}
			}
			if !approved {
				return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
			}
			continue
		}
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]any)
			if !ok {
				return biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED")
			}
			danger, e := dangerousRule(rule, b.GetKind() == "ClusterRoleBinding", i.p.config.TrustedPrincipals)
			if e != nil {
				return e
			}
			if danger {
				resources, _ := stringSlice(rule["resources"])
				verbs, _ := stringSlice(rule["verbs"])
				i.proof.DangerousEdges = append(i.proof.DangerousEdges, biz.EnvironmentAuthorizationEdge{BindingKind: b.GetKind(), BindingName: b.GetName(), RoleKind: kind, RoleName: name, SubjectKind: s.Kind, SubjectName: s.Name, SubjectNamespace: s.Namespace, Resources: resources, Verbs: verbs})
				return biz.EnvironmentError("KFP_ARTIFACT_ACCESS_NOT_ISOLATED")
			}
		}
	}
	return nil
}
func stringSlice(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid rule")
	}
	out := []string{}
	for _, x := range a {
		s, ok := x.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("invalid rule")
		}
		out = append(out, s)
	}
	return out, nil
}
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s || x == "*" {
			return true
		}
	}
	return false
}
func matchesResource(xs []string, s string) bool {
	for _, x := range xs {
		if x == s || x == "*" {
			return true
		}
		if strings.HasPrefix(x, "*/") && strings.HasSuffix(s, x[1:]) {
			return true
		}
	}
	return false
}
func anyVerb(xs []string, wants ...string) bool {
	for _, s := range wants {
		if contains(xs, s) {
			return true
		}
	}
	return false
}
func dangerousRule(m map[string]any, global bool, trusted []Subject) (bool, error) {
	bad := func() (bool, error) { return false, biz.EnvironmentError("ENVIRONMENT_AUTHORIZATION_UNVERIFIED") }
	for k := range m {
		if k != "apiGroups" && k != "verbs" && k != "resources" && k != "resourceNames" && k != "nonResourceURLs" {
			return bad()
		}
	}
	groups := []string{}
	if raw, ok := m["apiGroups"]; ok {
		a, ok := raw.([]any)
		if !ok {
			return bad()
		}
		for _, v := range a {
			s, ok := v.(string)
			if !ok {
				return bad()
			}
			groups = append(groups, s)
		}
	}
	verbs, e := stringSlice(m["verbs"])
	if e != nil || len(verbs) == 0 {
		return bad()
	}
	resources, e := stringSlice(m["resources"])
	if e != nil {
		return bad()
	}
	names, e := stringSlice(m["resourceNames"])
	if e != nil {
		return bad()
	}
	urls, e := stringSlice(m["nonResourceURLs"])
	if e != nil {
		return bad()
	}
	if len(urls) > 0 {
		if len(resources) > 0 || len(groups) > 0 {
			return bad()
		}
		return false, nil
	}
	if len(resources) == 0 || len(groups) == 0 {
		return bad()
	}
	for _, r := range resources {
		if strings.Contains(r, "*") && r != "*" && !(strings.HasPrefix(r, "*/") && strings.Count(r, "*") == 1 && strings.Count(r, "/") == 1) {
			return bad()
		}
	}
	if contains(groups, "") {
		if matchesResource(resources, "secrets") && anyVerb(verbs, "get", "list", "watch") && (len(names) == 0 || contains(names, "modeldev-kfp-artifacts")) {
			return true, nil
		}
		if (matchesResource(resources, "pods/exec") || matchesResource(resources, "pods/attach")) && anyVerb(verbs, "get", "create") {
			return true, nil
		}
		if matchesResource(resources, "serviceaccounts/token") && anyVerb(verbs, "create") {
			reserved := []string{"modeldev-step", "trainer-workload", "verifier"}
			if global {
				for _, s := range trusted {
					if s.Kind == "ServiceAccount" {
						reserved = append(reserved, s.Name)
					}
					if s.Kind == "User" && strings.HasPrefix(s.Name, "system:serviceaccount:") {
						parts := strings.Split(s.Name, ":")
						if len(parts) == 4 {
							reserved = append(reserved, parts[3])
						}
					}
				}
			}
			if len(names) == 0 {
				return true, nil
			}
			for _, name := range reserved {
				if contains(names, name) {
					return true, nil
				}
			}
		}
	}
	if (contains(groups, "") || contains(groups, "authentication.k8s.io")) && anyVerb(verbs, "impersonate") {
		for _, r := range []string{"users", "groups", "serviceaccounts", "userextras", "userextras/scopes", "uids"} {
			if matchesResource(resources, r) {
				return true, nil
			}
		}
	}
	if contains(groups, "rbac.authorization.k8s.io") && anyVerb(verbs, "bind", "escalate") {
		return true, nil
	}
	return false, nil
}
