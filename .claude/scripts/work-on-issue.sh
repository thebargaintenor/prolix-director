#!/bin/bash

set -euo pipefail

source ~/.claude/.env

usage() {
	echo "Usage: $0 [--main-branch MAIN_BRANCH_NAME] [--skip-pipeline] [--git-provider <GIT_PROVIDER>] ISSUE_NUM"
	exit 1
}

SKIP_PIPELINE=

while [[ $# -gt 0 ]]; do
	case "$1" in
		--main-branch)
			MAIN_BRANCH_PARAM=$2
			shift 2
			;;
		--skip-pipeline)
			SKIP_PIPELINE=true
			shift
			;;
		--git-provider)
			GIT_PROVIDER=$2
			shift 2
			;;
		*)
			break
			;;
	esac
done

if [[ $# -ne 1 ]]; then
		usage
fi

ISSUE_NUM=${1:?usage}
MAIN_BRANCH=${MAIN_BRANCH_PARAM:-trunk}
BASE_WORKTREE_PATH="$HOME/.config/ai-worktrees/$(basename "$PWD")"
WORKTREE_PATH="$BASE_WORKTREE_PATH/agent-issue-$ISSUE_NUM"
MAIN_SESSION_ID=$(uuidgen)
echo "Main session ID: $MAIN_SESSION_ID"
REVIEWER_SESSION_ID=$(uuidgen)
echo "Reviewer session ID: $REVIEWER_SESSION_ID"
MR_NUM=
if [[ "$GIT_PROVIDER" == "gitlab" ]]; then
	MR_OR_PR="MR"
elif [[ "$GIT_PROVIDER" == "github" ]]; then
	MR_OR_PR="PR"
else
	echo "$GIT_PROVIDER is unsupported at this time"
	exit 1
fi

IMPL_JSON_SCHEMA=$(cat <<EOF
{"type":"object","properties":{"${MR_OR_PR}_number":{"type":"integer"},"clarifying_question":{"type":"string"}}}
EOF
)

echo "Creating worktree"
mkdir -p "$BASE_WORKTREE_PATH"
git checkout $MAIN_BRANCH
git pull
git worktree add -b "agent-issue-$ISSUE_NUM" "$WORKTREE_PATH"
cd "$WORKTREE_PATH"

function cleanup() {
	set +e
	git push || true
	cd - || true
	git worktree remove "$WORKTREE_PATH"
	git checkout $MAIN_BRANCH
	git pull
	git branch -D "agent-issue-$ISSUE_NUM"
}

function claude_error_handling() {
	local -a cmd=("$@")
	local retried=false
	while true; do
		if [[ "$retried" == true ]]; then
			local -a new_cmd=()
			local skip_next=false
			for arg in "${cmd[@]}"; do
				if [[ "$skip_next" == true ]]; then
					skip_next=false
					continue
				fi
				if [[ "$arg" == "--session-id" ]]; then
					new_cmd+=("--resume")
					skip_next=false
				else
					new_cmd+=("$arg")
				fi
			done
			cmd=("${new_cmd[@]}")
		fi
		RESPONSE=$("${cmd[@]}") || true
		cat <<EOF
===========================
|| 🤖 Claude Response 🤖 ||
===========================
EOF
		RESULT=$(echo "$RESPONSE" | jq -r '.result // empty')
		echo "$RESULT"
		echo ""
		if [[ "$(echo "$RESPONSE" | jq -r '.is_error')" == "true" ]]; then
			RESULT=$(echo "$RESPONSE" | jq -r '.result // empty')
			echo "Error output from Claude: $RESULT"
			if [[ "$RESULT" == *"You've hit your limit"* ]]; then
				CLEAN_TIME=$(echo "$RESULT" | sed 's/.*resets //' | tr -d '(),')
				echo "extracted $CLEAN_TIME as reset time"
				CURRENT_EPOCH=$(date +%s)
				TARGET_EPOCH=$(date -d "$CLEAN_TIME" +%s)
				DIFF_IN_SECONDS=$((TARGET_EPOCH - CURRENT_EPOCH + 60))
				if [ $DIFF_IN_SECONDS -lt 0 ]; then
					TARGET_EPOCH=$(date -d "tomorrow $CLEAN_TIME" +%s)
					DIFF_IN_SECONDS=$((TARGET_EPOCH - CURRENT_EPOCH + 60))
				fi

				echo ""
				while [ $DIFF_IN_SECONDS -gt 0 ]; do
					printf '\rWaiting %(%H hours, %M minutes, and %S)T seconds ...' $DIFF_IN_SECONDS
					sleep 1
					: $((DIFF_IN_SECONDS--))
				done
				printf "\n*Yawns*... Okay, time to get up.\n"
			else
				read -rp "continue?[yn] " USER_ANSWER
				if [ $USER_ANSWER == "n" ]; then
					exit 1
				fi
			fi
			retried=true
		else
			break
		fi
	done
}

trap cleanup EXIT

echo "===== Phase 1: Implementation ====="
echo "Working on issue..."
claude_error_handling claude -p --dangerously-skip-permissions --session-id "$MAIN_SESSION_ID" --output-format json --model "$CODE_GEN_MODEL" --json-schema "$IMPL_JSON_SCHEMA" "/test-driven-development Work on $GIT_PROVIDER issue $ISSUE_NUM. If you don't have enough context to create a $MR_OR_PR, output a clarifying question via the \`clarifying question\` JSON key. Otherwise, create a $MR_OR_PR and output the number via the \`$MR_OR_PR number\` JSON key"

# do QA in a loop if necessary
while [[ -z "$MR_NUM" ]]; do
	while [ "$(echo "$RESPONSE" | jq 'has("structured_output")')" = "false" ]; do
		echo "AI response did not contain structured output -- re-prompting"
		claude_error_handling claude -p --dangerously-skip-permissions --resume "$MAIN_SESSION_ID" --output-format json --model "$CODE_GEN_MODEL" --json-schema "$IMPL_JSON_SCHEMA" "Your last response didn't provide an $MR_OR_PR number or a clarifying question."
	done
	MR_NUM=$(echo "$RESPONSE" | jq -r --arg key "${MR_OR_PR}_number" '.structured_output[$key] // empty')
	QUESTION=$(echo "$RESPONSE" | jq -r '.structured_output.clarifying_question // empty')
	if [[ -n "$QUESTION" ]]; then
		echo "===== Clarification Question Asked ====="
		echo "$QUESTION"
		read -rp "> " USER_ANSWER
		echo "Working on issue..."
		claude_error_handling claude --resume "$MAIN_SESSION_ID" -p --dangerously-skip-permissions --output-format json --model "$CODE_GEN_MODEL" --json-schema "$IMPL_JSON_SCHEMA" "The user has answered your clarifying question with $USER_ANSWER"
		MR_NUM=$(echo "$RESPONSE" | jq -r --arg key "${MR_OR_PR}_number" '.structured_output[$key] // empty')
		QUESTION=$(echo "$RESPONSE" | jq -r '.structured_output.clarifying_question // empty')
	fi
done

function glab_pipeline_loop() {
	PIPELINE_STATUS=$(glab mr view "$MR_NUM" --output json | jq -r '.head_pipeline.status')
	echo "Waiting on pipeline..."
	echo ""
	while [[ "$PIPELINE_STATUS" != "success" ]]; do
		while [[ "$PIPELINE_STATUS" != "success" && "$PIPELINE_STATUS" != "failed" && "$PIPELINE_STATUS" != "canceled" && "$PIPELINE_STATUS" != "skipped" ]]; do
			printf "\rCurrent status: %s" "$PIPELINE_STATUS"
			sleep 60
			PIPELINE_STATUS=$(glab mr view "$MR_NUM" --output json | jq -r '.head_pipeline.status')
		done
		printf "\nPipeline finished with status $PIPELINE_STATUS\n"
		if [[ "$PIPELINE_STATUS" == "failed" ]]; then
			claude_error_handling claude --resume "$MAIN_SESSION_ID" -p --dangerously-skip-permissions --output-format json --model "$CODE_GEN_MODEL" "Looks like your pipeline failed. Please fix and push"
		elif [[ "$PIPELINE_STATUS" == "success" ]]; then
			break
		else
			read -rp "The pipeline status is $PIPELINE_STATUS, continue?[yn] " USER_ANSWER
			if [[ "$USER_ANSWER" == "n" ]]; then
				exit 0
			fi
		fi
		PIPELINE_STATUS=$(glab mr view "$MR_NUM" --output json | jq -r '.head_pipeline.status')
	done
}

function gh_pipeline_loop() {
	local max_attempts=3
	local attempt=0
	until gh pr checks $MR_NUM --watch; do
		attempt=$((attempt + 1))
		if [[ $attempt -ge $max_attempts ]]; then
			echo "Pipeline failed $max_attempts times. Requesting human input."
			read -rp "Provide guidance or 'q' to quit: " USER_ANSWER
			if [[ "$USER_ANSWER" == "q" ]]; then
				exit 1
			fi
			claude_error_handling claude --resume "$MAIN_SESSION_ID" -p --dangerously-skip-permissions --output-format json --model "$CODE_GEN_MODEL" "The pipeline has failed $max_attempts times. The user says: $USER_ANSWER"
			attempt=0
		else
			echo "Prompting claude to fix pipeline (attempt $attempt/$max_attempts)..."
			claude_error_handling claude --resume "$MAIN_SESSION_ID" -p --dangerously-skip-permissions --output-format json --model "$CODE_GEN_MODEL" "Looks like your pipeline failed. Please fix and push"
		fi
	done
}

pipeline_loop_wrapper() {
	if [ -z "$SKIP_PIPELINE" ]; then
		if [[ "$GIT_PROVIDER" == "gitlab" ]]; then
			glab_pipeline_loop
		elif [[ "$GIT_PROVIDER" == "github" ]]; then
			gh_pipeline_loop
		else
			echo "$GIT_PROVIDER is unsupported at this time"
			exit 1
		fi
	else
		echo "--skip-pipeline was set. Skipping pipeline loop..."
	fi
}

echo "===== Phase 2: Pipeline loop 1 ====="
pipeline_loop_wrapper

echo "===== Phase 3: AI Code Review ====="
echo "Reviewing..."
claude_error_handling claude -p --output-format json --dangerously-skip-permissions --model "$REVIEWER_MODEL" --session-id "$REVIEWER_SESSION_ID" "/code-review $MR_OR_PR $MR_NUM Leave your review as a comment on the $MR_OR_PR"

echo "===== Phase 4: Address CR comments ====="
echo "Addressing comments..."
claude_error_handling claude -p --output-format json --dangerously-skip-permissions --model "$CODE_GEN_MODEL" --resume "$MAIN_SESSION_ID" "Read the review comments on $MR_OR_PR $MR_NUM and make changes accordingly"

echo "===== Phase 5: Pipeline loop 2 ====="
pipeline_loop_wrapper

echo "===== Phase 6: Human Review Loop ====="
while true; do
	echo -e "Please provide your code review.\n1. Cleanup\n2. Address comments\n3. Rewrite Issue"
	read -rp "> " USER_ANSWER
	if [[ "$USER_ANSWER" == "1" ]]; then
		exit 0
	elif [[ "$USER_ANSWER" == "2" ]]; then
		echo "Addressing additional comments..."
		claude_error_handling claude -p --output-format json --dangerously-skip-permissions --model "$CODE_GEN_MODEL" --resume "$MAIN_SESSION_ID" "Read the review latest comments on $MR_OR_PR $MR_NUM and make changes accordingly"
		pipeline_loop_wrapper
	elif [[ "$USER_ANSWER" == "3" ]]; then
		echo "Summarizing work into issue comment..."
		claude_error_handling claude -p --output-format json --dangerously-skip-permissions --model "$CODE_GEN_MODEL" --resume "$MAIN_SESSION_ID" "The user has decided to close the $MR_OR_PR. Leave a summary of this session as a comment on issue $ISSUE_NUM"
		exit 0
	else
		echo "Not a valid choice"
	fi
done
