package workflowtemplate

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowtemplatepkg "github.com/argoproj/argo-workflows/v3/pkg/apiclient/workflowtemplate"
	"github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/argoproj/argo-workflows/v3/server/auth"
	sutils "github.com/argoproj/argo-workflows/v3/server/utils"
	"github.com/argoproj/argo-workflows/v3/util/instanceid"
	"github.com/argoproj/argo-workflows/v3/workflow/creator"
	"github.com/argoproj/argo-workflows/v3/workflow/templateresolution"
	"github.com/argoproj/argo-workflows/v3/workflow/validate"
)

type WorkflowTemplateServer struct {
	instanceIDService instanceid.Service
}

func NewWorkflowTemplateServer(instanceIDService instanceid.Service) workflowtemplatepkg.WorkflowTemplateServiceServer {
	return &WorkflowTemplateServer{instanceIDService}
}

func (wts *WorkflowTemplateServer) CreateWorkflowTemplate(ctx context.Context, req *workflowtemplatepkg.WorkflowTemplateCreateRequest) (*v1alpha1.WorkflowTemplate, error) {
	wfClient := auth.GetWfClient(ctx)
	if req.Template == nil {
		return nil, sutils.ToStatusError(fmt.Errorf("workflow template was not found in the request body"), codes.InvalidArgument)
	}
	wts.instanceIDService.Label(req.Template)
	creator.Label(ctx, req.Template)
	wftmplGetter := templateresolution.WrapWorkflowTemplateInterface(wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace))
	cwftmplGetter := templateresolution.WrapClusterWorkflowTemplateInterface(wfClient.ArgoprojV1alpha1().ClusterWorkflowTemplates())
	err := validate.ValidateWorkflowTemplate(wftmplGetter, cwftmplGetter, req.Template, validate.ValidateOpts{})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.InvalidArgument)
	}
	wfTmpl, err := wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace).Create(ctx, req.Template, v1.CreateOptions{})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.InvalidArgument)
	}
	return wfTmpl, nil
}

func (wts *WorkflowTemplateServer) GetWorkflowTemplate(ctx context.Context, req *workflowtemplatepkg.WorkflowTemplateGetRequest) (*v1alpha1.WorkflowTemplate, error) {
	wfTmpl, err := wts.getTemplateAndValidate(ctx, req.Namespace, req.Name)
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.Internal)
	}
	return wfTmpl, nil
}

func (wts *WorkflowTemplateServer) getTemplateAndValidate(ctx context.Context, namespace string, name string) (*v1alpha1.WorkflowTemplate, error) {
	wfClient := auth.GetWfClient(ctx)
	wfTmpl, err := wfClient.ArgoprojV1alpha1().WorkflowTemplates(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.Internal)
	}
	err = wts.instanceIDService.Validate(wfTmpl)
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.InvalidArgument)
	}
	return wfTmpl, nil
}

func (wts *WorkflowTemplateServer) ListWorkflowTemplates(ctx context.Context, req *workflowtemplatepkg.WorkflowTemplateListRequest) (*v1alpha1.WorkflowTemplateList, error) {
	wfClient := auth.GetWfClient(ctx)
	options := &v1.ListOptions{}
	if req.ListOptions != nil {
		options = req.ListOptions
	}

	// kubernetes api will search for all result.
	// Search whole with limit 0 and save the original limit for custom filtering.
	limit := options.Limit
	options.Limit = 0

	// Continue is not used for kubernetes api search offset which is base64.
	// Now it is just simple number as a string, which is used for custom filtering.
	var err error
	intOffset := 0
	if options.Continue != "" {
		intOffset, err = strconv.Atoi(options.Continue)
		if err != nil {
			return nil, sutils.ToStatusError(fmt.Errorf("invalid offset format: %s", options.Continue), codes.InvalidArgument)
		}
		// Prevent continue applied to kubernetes api
		options.Continue = ""
	}

	wts.instanceIDService.With(options)
	wfList, err := wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace).List(ctx, *options)
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.Internal)
	}

	items := []v1alpha1.WorkflowTemplate{}

	// Do name pattern filtering if exist
	if req.NamePattern != "" {
		for _, item := range wfList.Items {
			if strings.Contains(item.ObjectMeta.Name, req.NamePattern) {
				items = append(items, item)
			}
		}
	} else {
		items = wfList.Items
	}

	// Apply offset and limit
	startIndex := intOffset
	if startIndex > len(items) {
		startIndex = len(items)
	}

	endIndex := startIndex + int(limit)
	if endIndex > len(items) || limit == 0 {
		endIndex = len(items)
	}

	wfList.Items = items[startIndex:endIndex]

	sort.Sort(wfList.Items)

	return wfList, nil
}

func (wts *WorkflowTemplateServer) DeleteWorkflowTemplate(ctx context.Context, req *workflowtemplatepkg.WorkflowTemplateDeleteRequest) (*workflowtemplatepkg.WorkflowTemplateDeleteResponse, error) {
	wfClient := auth.GetWfClient(ctx)
	_, err := wts.getTemplateAndValidate(ctx, req.Namespace, req.Name)
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.Internal)
	}
	err = wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace).Delete(ctx, req.Name, v1.DeleteOptions{})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.Internal)
	}
	return &workflowtemplatepkg.WorkflowTemplateDeleteResponse{}, nil
}

func (wts *WorkflowTemplateServer) LintWorkflowTemplate(ctx context.Context, req *workflowtemplatepkg.WorkflowTemplateLintRequest) (*v1alpha1.WorkflowTemplate, error) {
	wfClient := auth.GetWfClient(ctx)
	wts.instanceIDService.Label(req.Template)
	creator.Label(ctx, req.Template)
	wftmplGetter := templateresolution.WrapWorkflowTemplateInterface(wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace))
	cwftmplGetter := templateresolution.WrapClusterWorkflowTemplateInterface(wfClient.ArgoprojV1alpha1().ClusterWorkflowTemplates())
	err := validate.ValidateWorkflowTemplate(wftmplGetter, cwftmplGetter, req.Template, validate.ValidateOpts{Lint: true})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.InvalidArgument)
	}
	return req.Template, nil
}

func (wts *WorkflowTemplateServer) UpdateWorkflowTemplate(ctx context.Context, req *workflowtemplatepkg.WorkflowTemplateUpdateRequest) (*v1alpha1.WorkflowTemplate, error) {
	if req.Template == nil {
		return nil, sutils.ToStatusError(fmt.Errorf("WorkflowTemplate is not found in Request body"), codes.InvalidArgument)
	}
	err := wts.instanceIDService.Validate(req.Template)
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.InvalidArgument)
	}
	wfClient := auth.GetWfClient(ctx)
	wftmplGetter := templateresolution.WrapWorkflowTemplateInterface(wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace))
	cwftmplGetter := templateresolution.WrapClusterWorkflowTemplateInterface(wfClient.ArgoprojV1alpha1().ClusterWorkflowTemplates())
	err = validate.ValidateWorkflowTemplate(wftmplGetter, cwftmplGetter, req.Template, validate.ValidateOpts{})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.InvalidArgument)
	}
	res, err := wfClient.ArgoprojV1alpha1().WorkflowTemplates(req.Namespace).Update(ctx, req.Template, v1.UpdateOptions{})
	if err != nil {
		return nil, sutils.ToStatusError(err, codes.Internal)
	}
	return res, nil
}
