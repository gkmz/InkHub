package bootstrap

import "github.com/gkmz/InkHub/internal/domain/article"

type articleWorkflowInput struct {
	ContentStage      article.ContentStage
	ContentIssue      string
	ReviewState       string
	ApprovedHash      string
	ContentHash       string
	Disposition       string
	DispositionHash   string
	HugoState         string
	HugoHash          string
	WeChatState       string
	WeChatHash        string
	AvailableChannels []string
}

type articleWorkflowResult struct {
	State       string
	HugoLabel   string
	WeChatLabel string
	NextAction  string
	Bucket      string
}

// deriveArticleWorkflow 根据内容阶段、审核版本和渠道投影推导用户可见工作流状态。
func deriveArticleWorkflow(input articleWorkflowInput) articleWorkflowResult {
	result := articleWorkflowResult{
		HugoLabel:   publicationLabel(effectivePublicationState("hugo", input.HugoState, input.HugoHash, input.ContentHash), "hugo"),
		WeChatLabel: publicationLabel(effectivePublicationState("wechat", input.WeChatState, input.WeChatHash, input.ContentHash), "wechat"),
	}
	if input.ContentStage != article.ContentStageReady {
		result.State = "draft"
		result.HugoLabel = "未进入发布流程"
		result.WeChatLabel = "未进入发布流程"
		return result
	}
	if currentWorkflowFailure(input) {
		result.State, result.NextAction, result.Bucket = "blocked", "retry", "failed"
		return result
	}
	if currentWorkflowChanged(input) {
		result.State, result.NextAction, result.Bucket = "changed", "review", "changed"
		return result
	}
	// 已标记为当前版本已发表的历史文章不需要凭空补造审核记录。
	if input.Disposition == "published" && input.DispositionHash == input.ContentHash {
		result.State, result.NextAction, result.Bucket = "approved", "view", "recently_handled"
		return result
	}
	if needsReviewState(input.ReviewState) || input.ReviewState == "" {
		result.State, result.NextAction, result.Bucket = "pending_review", "review", "needs_review"
		return result
	}
	if input.ReviewState == "approved" && input.ApprovedHash == input.ContentHash {
		result.State, result.NextAction = "approved", "publish"
		if input.Disposition == "published" {
			result.NextAction, result.Bucket = "view", "recently_handled"
		} else if !hasPendingChannel(input) {
			result.NextAction, result.Bucket = "view", "latest_ready"
		} else {
			result.Bucket = "ready_to_publish"
		}
		return result
	}
	result.State, result.NextAction, result.Bucket = "pending_review", "review", "needs_review"
	return result
}

// hasPendingChannel 判断当前已启用渠道是否仍需要处理当前内容版本。
func hasPendingChannel(input articleWorkflowInput) bool {
	for _, channel := range input.AvailableChannels {
		switch channel {
		case "hugo":
			if input.HugoState != "published" || input.HugoHash != input.ContentHash {
				return true
			}
		case "wechat":
			// 微信人工确认是终态，确认后正文变化也不重复创建任务。
			if input.WeChatState != "confirmed" && (input.WeChatState != "published" || input.WeChatHash != input.ContentHash) {
				return true
			}
		}
	}
	return false
}

func currentWorkflowFailure(input articleWorkflowInput) bool {
	return input.ReviewState == "blocked" || publicationMatchesString(input.HugoState, input.HugoHash, "failed", input.ContentHash) || publicationMatchesString(input.WeChatState, input.WeChatHash, "failed", input.ContentHash)
}

func publicationMatchesString(state, hash, wantState, currentHash string) bool {
	return state == wantState && hash == currentHash
}

func currentWorkflowChanged(input articleWorkflowInput) bool {
	if input.ReviewState == "changed" || (input.ApprovedHash != "" && input.ApprovedHash != input.ContentHash) {
		return true
	}
	if input.Disposition == "published" && input.DispositionHash != input.ContentHash {
		return true
	}
	return publicationOutdatedForWorkflow("hugo", input.HugoState, input.HugoHash, input.ContentHash) || publicationOutdatedForWorkflow("wechat", input.WeChatState, input.WeChatHash, input.ContentHash)
}

func publicationOutdatedForWorkflow(channel, state, storedHash, currentHash string) bool {
	return state != "" && !(channel == "wechat" && state == "confirmed") && storedHash != currentHash
}

func effectivePublicationState(channel, state, storedHash, currentHash string) string {
	if state == "" || (channel == "wechat" && state == "confirmed") {
		return state
	}
	if storedHash != currentHash {
		return "outdated"
	}
	return state
}

func needsReviewState(state string) bool {
	switch state {
	case "draft", "incomplete", "pending_review", "changed":
		return true
	default:
		return false
	}
}
