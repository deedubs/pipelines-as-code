package gitlab

import (
	"context"
	"fmt"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/acl"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/params/info"
	"github.com/xanzy/go-gitlab"
)

// get the owner file from main branch and check if we are allowing there
func (v *Provider) isAllowedFromOwnerFile(event *info.Event) bool {
	ownerContent, _ := v.getObject("OWNERS", event.DefaultBranch, v.targetProjectID)
	if string(ownerContent) == "" {
		return false
	}
	allowed, _ := acl.UserInOwnerFile(string(ownerContent), event.Sender)
	return allowed
}

func (v *Provider) checkMembership(event *info.Event, userid int) bool {
	member, _, err := v.Client.ProjectMembers.GetInheritedProjectMember(v.targetProjectID, userid)
	if err == nil && member.ID == userid {
		return true
	}

	return v.isAllowedFromOwnerFile(event)
}

func (v *Provider) checkOkToTestCommentFromApprovedMember(event *info.Event) (bool, error) {
	// TODO: we need to handle pagination :\
	opt := &gitlab.ListMergeRequestDiscussionsOptions{}
	discussions, _, err := v.Client.Discussions.ListMergeRequestDiscussions(v.targetProjectID, v.mergeRequestID, opt)
	if err != nil {
		return false, err
	}

	for _, comment := range discussions {
		// TODO: maybe we do threads in the future but for now we just check the top thread for ops related comments
		topthread := comment.Notes[0]
		if acl.MatchRegexp(acl.OKToTestCommentRegexp, topthread.Body) {
			commenterEvent := &info.Event{
				Event:         event.Event,
				Sender:        topthread.Author.Username,
				BaseBranch:    event.BaseBranch,
				HeadBranch:    event.HeadBranch,
				Repository:    event.Repository,
				Organization:  event.Organization,
				DefaultBranch: event.DefaultBranch,
			}
			// TODO: we could probably do with caching when checking all issues?
			if v.checkMembership(commenterEvent, topthread.Author.ID) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (v *Provider) IsAllowed(ctx context.Context, event *info.Event) (bool, error) {
	if v.Client == nil {
		return false, fmt.Errorf("no github client has been initiliazed, " +
			"exiting... (hint: did you forget setting a secret on your repo?)")
	}
	if v.checkMembership(event, v.userID) {
		return true, nil
	}

	return v.checkOkToTestCommentFromApprovedMember(event)
}
