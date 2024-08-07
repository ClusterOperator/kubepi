package cluster

import (
	goContext "context"
	"errors"
	"fmt"
	"strings"

	"github.com/ClusterOperator/kubepi/internal/api/v1/session"
	v1 "github.com/ClusterOperator/kubepi/internal/model/v1"
	v1Cluster "github.com/ClusterOperator/kubepi/internal/model/v1/cluster"
	"github.com/ClusterOperator/kubepi/internal/server"
	"github.com/ClusterOperator/kubepi/internal/service/v1/common"
	"github.com/ClusterOperator/kubepi/pkg/collectons"
	"github.com/ClusterOperator/kubepi/pkg/kubernetes"
	"github.com/asdine/storm/v3"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Update Cluster Member
// @Tags clusters
// @Summary Update Cluster Member
// @Description Update Cluster Member
// @Accept  json
// @Produce  json
// @Param cluster path string true "集群名称"
// @Param member path string true "成员名称"
// @Param request body Member true "request"
// @Success 200 {object} Member
// @Security ApiKeyAuth
// @Router /clusters/{cluster}/members/{member} [put]
func (h *Handler) UpdateClusterMember() iris.Handler {
	return func(ctx *context.Context) {
		name := ctx.Params().GetString("name")
		var req Member
		err := ctx.ReadJSON(&req)
		if err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.Values().Set("message", fmt.Sprintf("delete cluster failed: %s", err.Error()))
			return
		}
		c, err := h.clusterService.Get(name, common.DBOptions{})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster failed: %s", err.Error()))
			return
		}
		if c.CreatedBy == req.Name {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.Values().Set("message", fmt.Sprintf("can not delete or update cluster importer %s", req.Name))
			return
		}
		k := kubernetes.NewKubernetes(c)
		if err := k.CleanManagedClusterRoleBinding(req.Name); err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", err)
			return
		}
		if err := k.CleanManagedRoleBinding(req.Name); err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", err)
			return
		}
		// 删除重建
		for i := range req.NamespaceRoles {
			for j := range req.NamespaceRoles[i].Roles {
				if err := k.CreateOrUpdateRolebinding(req.NamespaceRoles[i].Namespace, req.NamespaceRoles[i].Roles[j], req.Name, false); err != nil {
					ctx.StatusCode(iris.StatusInternalServerError)
					ctx.Values().Set("message", err)
					return
				}
			}
		}
		for i := range req.ClusterRoles {
			if err := k.CreateOrUpdateClusterRoleBinding(req.ClusterRoles[i], req.Name, false); err != nil {
				ctx.StatusCode(iris.StatusInternalServerError)
				ctx.Values().Set("message", err)
				return
			}
		}
		ctx.Values().Set("data", &req)
	}
}

// Get Cluster Member By name
// @Tags clusters
// @Summary Get Cluster Member By name
// @Description Get Cluster Member By name
// @Accept  json
// @Produce  json
// @Param cluster path string true "集群名称"
// @Param member path string true "成员名称"
// @Success 200 {object} Member
// @Security ApiKeyAuth
// @Router /clusters/{cluster}/members/{member} [get]
func (h *Handler) GetClusterMember() iris.Handler {
	return func(ctx *context.Context) {
		name := ctx.Params().GetString("name")
		memberName := ctx.Params().Get("member")

		c, err := h.clusterService.Get(name, common.DBOptions{})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster failed: %s", err.Error()))
			return
		}
		k := kubernetes.NewKubernetes(c)
		client, err := k.Client()
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get k8s client failed: %s", err.Error()))
			return
		}

		binding, err := h.clusterBindingService.GetBindingByClusterNameAndUserName(name, memberName, common.DBOptions{})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster binding failed: %s", err.Error()))
			return
		}
		labels := []string{
			fmt.Sprintf("%s=%s", kubernetes.LabelManageKey, "kubepi"),
			fmt.Sprintf("%s=%s", kubernetes.LabelClusterId, c.UUID),
			fmt.Sprintf("%s=%s", kubernetes.LabelUsername, binding.UserRef),
		}
		clusterRoleBindings, err := client.RbacV1().ClusterRoleBindings().List(goContext.TODO(), metav1.ListOptions{
			LabelSelector: strings.Join(labels, ","),
		})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", err)
			return
		}
		rolebindings, err := client.RbacV1().RoleBindings("").List(goContext.TODO(), metav1.ListOptions{
			LabelSelector: strings.Join(labels, ","),
		})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", err)
			return
		}

		var member Member
		member.ClusterRoles = make([]string, 0)
		member.NamespaceRoles = make([]NamespaceRoles, 0)
		member.Name = binding.UserRef
		set := collectons.NewStringSet()
		for i := range clusterRoleBindings.Items {
			set.Add(clusterRoleBindings.Items[i].RoleRef.Name)
		}
		member.ClusterRoles = set.ToSlice()

		roleMap := map[string][]string{}

		for i := range rolebindings.Items {
			if roleMap[rolebindings.Items[i].Namespace] == nil {
				roleMap[rolebindings.Items[i].Namespace] = []string{rolebindings.Items[i].RoleRef.Name}
			} else {
				roleMap[rolebindings.Items[i].Namespace] = append(roleMap[rolebindings.Items[i].Namespace], rolebindings.Items[i].RoleRef.Name)
			}
		}
		for k := range roleMap {
			member.NamespaceRoles = append(member.NamespaceRoles, NamespaceRoles{
				Namespace: k,
				Roles:     roleMap[k],
			})
		}
		ctx.Values().Set("data", &member)
	}

}

// List ClusterMembers
// @Tags clusters
// @Summary List all ClusterMembers
// @Description List all ClusterMembers
// @Accept  json
// @Produce  json
// @Param cluster path string true "集群名称"
// @Success 200 {object} []Member
// @Security ApiKeyAuth
// @Router /clusters/{cluster}/members [get]
func (h *Handler) ListClusterMembers() iris.Handler {
	return func(ctx *context.Context) {
		name := ctx.Params().GetString("name")
		bindings, err := h.clusterBindingService.GetClusterBindingByClusterName(name, common.DBOptions{})
		if err != nil && !errors.Is(err, storm.ErrNotFound) {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", err.Error())
			return
		}
		members := make([]Member, 0)
		for i := range bindings {
			members = append(members, Member{
				Name:        bindings[i].UserRef,
				BindingName: bindings[i].Name,
				CreateAt:    bindings[i].CreateAt,
			})
		}
		ctx.Values().Set("data", members)
	}
}

// Create Cluster Member
// @Tags clusters
// @Summary Create Cluster Member
// @Description Create Cluster Member
// @Accept  json
// @Produce  json
// @Param cluster path string true "集群名称"
// @Param request body Member true "request"
// @Success 200 {object} Member
// @Security ApiKeyAuth
// @Router /clusters/{cluster}/members [post]
func (h *Handler) CreateClusterMember() iris.Handler {
	return func(ctx *context.Context) {
		name := ctx.Params().GetString("name")
		var req Member
		err := ctx.ReadJSON(&req)
		if err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.Values().Set("message", err.Error())
			return
		}
		if req.Name == "" {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.Values().Set("message", "username can not be none")
			return
		}
		if len(req.ClusterRoles) == 0 && len(req.NamespaceRoles) == 0 {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.Values().Set("message", "must select one role")
			return
		}
		u := ctx.Values().Get("profile")
		profile := u.(session.UserProfile)
		binding := v1Cluster.Binding{
			BaseModel: v1.BaseModel{
				Kind:      "ClusterBinding",
				CreatedBy: profile.Name,
			},
			Metadata: v1.Metadata{
				Name: fmt.Sprintf("%s-%s-cluster-binding", name, req.Name),
			},
			UserRef:    req.Name,
			ClusterRef: name,
		}

		tx, _ := server.DB().Begin(true)
		c, err := h.clusterService.Get(name, common.DBOptions{DB: tx})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster failed: %s", err.Error()))
			return
		}

		k := kubernetes.NewKubernetes(c)
		cert, err := k.CreateCommonUser(req.Name)
		if err != nil {
			_ = tx.Rollback()
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("create common user failed: %s", err.Error()))
			return
		}
		binding.Certificate = cert
		if err := h.clusterBindingService.CreateClusterBinding(&binding, common.DBOptions{DB: tx}); err != nil {
			_ = tx.Rollback()
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", "unable to complete authorization")
			return
		}
		// 创建clusterrolebinding
		for i := range req.ClusterRoles {
			if err := k.CreateOrUpdateClusterRoleBinding(req.ClusterRoles[i], req.Name, false); err != nil {
				_ = tx.Rollback()
				ctx.StatusCode(iris.StatusInternalServerError)
				ctx.Values().Set("message", "unable to complete authorization")
				return
			}
		}
		// 创建Rolebinding
		for i := range req.NamespaceRoles {
			for j := range req.NamespaceRoles[i].Roles {
				if err := k.CreateOrUpdateRolebinding(req.NamespaceRoles[i].Namespace, req.NamespaceRoles[i].Roles[j], req.Name, false); err != nil {
					_ = tx.Rollback()
					ctx.StatusCode(iris.StatusInternalServerError)
					ctx.Values().Set("message", err.Error())
					return
				}
			}
		}
		_ = tx.Commit()
		ctx.Values().Set("data", req)
	}
}

// Delete ClusterMember
// @Tags clusters
// @Summary Delete clusterMember by name
// @Description Delete clusterMember by name
// @Accept  json
// @Produce  json
// @Param cluster path string true "集群名称"
// @Param members path string true "成员名称"
// @Success 200 {number} 200
// @Security ApiKeyAuth
// @Router /clusters/{cluster}/members/{member} [delete]
func (h *Handler) DeleteClusterMember() iris.Handler {
	return func(ctx *context.Context) {
		name := ctx.Params().GetString("name")
		memberName := ctx.Params().GetString("member")
		u := ctx.Values().Get("profile")
		profile := u.(session.UserProfile)
		c, err := h.clusterService.Get(name, common.DBOptions{})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster failed: %s", err.Error()))
			return
		}
		if c.CreatedBy == memberName {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.Values().Set("message", fmt.Sprintf("can not delete or update cluster importer %s", profile.Name))
			return
		}

		binding, err := h.clusterBindingService.GetBindingByClusterNameAndUserName(c.Name, memberName, common.DBOptions{})
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster failed: %s", err.Error()))
			return
		}
		tx, err := server.DB().Begin(true)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("get cluster failed: %s", err.Error()))
			return
		}
		if err := h.clusterBindingService.Delete(binding.Name, common.DBOptions{DB: tx}); err != nil {
			_ = tx.Rollback()
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.Values().Set("message", fmt.Sprintf("delete cluster binding failed: %s", err.Error()))
			return
		}
		k := kubernetes.NewKubernetes(c)
		if err := k.CleanManagedClusterRoleBinding(memberName); err != nil {
			server.Logger().Errorf("can not delete cluster member %s : %s", memberName, err)
		}
		if err := k.CleanManagedRoleBinding(memberName); err != nil {
			server.Logger().Errorf("can not delete cluster member %s : %s", memberName, err)
		}
		_ = tx.Commit()
	}
}
