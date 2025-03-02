package pipelineascode

import (
	"strconv"
	"testing"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/keys"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/formatting"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/params"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/params/clients"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/params/info"
	testclient "github.com/openshift-pipelines/pipelines-as-code/pkg/test/clients"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"go.uber.org/zap"
	zapobserver "go.uber.org/zap/zaptest/observer"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	rtesting "knative.dev/pkg/reconciler/testing"
)

var (
	fooRepo = &v1alpha1.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
			Name:      "foo",
		},
		Spec: v1alpha1.RepositorySpec{
			URL: "https://github.com/fooorg/foo",
		},
	}
	fooRepoLabels = map[string]string{
		keys.URLRepository: formatting.K8LabelsCleanup("foo"),
		keys.SHA:           formatting.K8LabelsCleanup("foosha"),
		keys.PullRequest:   strconv.Itoa(11),
	}
	fooRepoLabelsPrFooAbc = map[string]string{
		keys.URLRepository:  formatting.K8LabelsCleanup("foo"),
		keys.SHA:            formatting.K8LabelsCleanup("foosha"),
		keys.PullRequest:    strconv.Itoa(11),
		keys.OriginalPRName: "pr-foo-abc",
	}
)

func TestCancelPipelinerun(t *testing.T) {
	observer, _ := zapobserver.New(zap.InfoLevel)
	logger := zap.New(observer).Sugar()
	tests := []struct {
		name                  string
		event                 *info.Event
		repo                  *v1alpha1.Repository
		pipelineRuns          []*pipelinev1beta1.PipelineRun
		cancelledPipelineRuns map[string]bool
	}{
		{
			name: "not a pull request event",
			event: &info.Event{
				TriggerTarget: "push",
			},
		},
		{
			name: "cancel running",
			event: &info.Event{
				Repository:        "foo",
				SHA:               "foosha",
				TriggerTarget:     "pull_request",
				PullRequestNumber: 11,
				State: info.State{
					CancelPipelineRuns: true,
				},
			},
			pipelineRuns: []*pipelinev1beta1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-foo",
						Namespace: "foo",
						Labels:    fooRepoLabels,
					},
					Spec: pipelinev1beta1.PipelineRunSpec{},
				},
			},
			repo: fooRepo,
			cancelledPipelineRuns: map[string]bool{
				"pr-foo": true,
			},
		},
		{
			name: "no pipelineruns found",
			event: &info.Event{
				Repository:        "foo",
				SHA:               "foosha",
				TriggerTarget:     "pull_request",
				PullRequestNumber: 11,
				State: info.State{
					CancelPipelineRuns: true,
				},
			},
			pipelineRuns:          []*pipelinev1beta1.PipelineRun{},
			repo:                  fooRepo,
			cancelledPipelineRuns: map[string]bool{},
		},
		{
			name: "cancel a specific run",
			event: &info.Event{
				Repository:        "foo",
				SHA:               "foosha",
				TriggerTarget:     "pull_request",
				PullRequestNumber: 11,
				State: info.State{
					CancelPipelineRuns:      true,
					TargetCancelPipelineRun: "pr-foo-abc",
				},
			},
			pipelineRuns: []*pipelinev1beta1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-foo",
						Namespace: "foo",
						Labels:    fooRepoLabels,
					},
					Spec: pipelinev1beta1.PipelineRunSpec{},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-foo-abc-123",
						Namespace: "foo",
						Labels:    fooRepoLabelsPrFooAbc,
					},
					Spec: pipelinev1beta1.PipelineRunSpec{},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-foo-pqr",
						Namespace: "foo",
						Labels:    fooRepoLabels,
					},
					Spec: pipelinev1beta1.PipelineRunSpec{},
				},
			},
			repo: fooRepo,
			cancelledPipelineRuns: map[string]bool{
				"pr-foo-abc-123": true,
			},
		},
		{
			name: "cancelling a done pipelinerun or already cancelled pipelinerun",
			event: &info.Event{
				Repository:        "foo",
				SHA:               "foosha",
				TriggerTarget:     "pull_request",
				PullRequestNumber: 11,
				State: info.State{
					CancelPipelineRuns: true,
				},
			},
			pipelineRuns: []*pipelinev1beta1.PipelineRun{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-foo",
						Namespace: "foo",
						Labels:    fooRepoLabels,
					},
					Spec: pipelinev1beta1.PipelineRunSpec{},
					Status: pipelinev1beta1.PipelineRunStatus{
						Status: duckv1beta1.Status{
							Conditions: duckv1beta1.Conditions{
								apis.Condition{
									Type:   apis.ConditionSucceeded,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pr-foo-1",
						Namespace: "foo",
						Labels:    fooRepoLabels,
					},
					Spec: pipelinev1beta1.PipelineRunSpec{
						Status: pipelinev1beta1.PipelineRunSpecStatusStoppedRunFinally,
					},
				},
			},
			repo:                  fooRepo,
			cancelledPipelineRuns: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := rtesting.SetupFakeContext(t)

			tdata := testclient.Data{
				PipelineRuns: tt.pipelineRuns,
			}
			stdata, _ := testclient.SeedTestData(t, ctx, tdata)
			cs := &params.Run{
				Clients: clients.Clients{
					Log:    logger,
					Tekton: stdata.Pipeline,
					Kube:   stdata.Kube,
				},
			}
			pac := NewPacs(tt.event, nil, cs, nil, logger)
			err := pac.cancelPipelineRuns(ctx, tt.repo)
			assert.NilError(t, err)

			got, err := cs.Clients.Tekton.TektonV1beta1().PipelineRuns("foo").List(ctx, metav1.ListOptions{})
			assert.NilError(t, err)

			for _, pr := range got.Items {
				// from the list only the ones which are in cancelled map should have cancel status
				if _, ok := tt.cancelledPipelineRuns[pr.Name]; ok {
					assert.Equal(t, string(pr.Spec.Status), pipelinev1beta1.PipelineRunSpecStatusCancelledRunFinally)
					continue
				}
				assert.Assert(t, string(pr.Spec.Status) != pipelinev1beta1.PipelineRunSpecStatusCancelledRunFinally)
			}
		})
	}
}
