# Delayed SQS (Step Functions) — Setup for Trendly

> ✅ **Status (02/06/2026): the CloudFormation resources below are now implemented**
> in `serverless.trendly.yml`, under these names:
> - SQS queue: **`ContentPublishQueue`** (`ContentPublishQueue-${stage}`)
> - State machine: **`ContentPublishStateMachine`** (`ContentPublishDelayedSQS-${stage}`)
> - State-machine role: **`ContentPublishStateMachineRole`**
> - Consumer Lambda: **`scheduled_publish_sqs`** (`functions/scheduled_publish_sqs/main.go`)
> - Env on `trendly_v2_apis`: `DELAY_STATE_FUNCTION` (state-machine ARN) + `SEND_MESSAGE_QUEUE_ARN` (`Ref ContentPublishQueue` → queue URL)
> - Provider IAM: `states:StartExecution`/`StopExecution` + `sqs:SendMessage` on the queue
>
> **What's left is operational, not code:** (1) `sls deploy` to provision them; (2) verify the
> deployed `trendly_v2_apis` Lambda shows both env vars; (3) smoke-test a 60s delayed publish;
> (4) complete **Meta App Review** for `instagram_business_content_publish` (Instagram flow) +
> `pages_manage_posts` (Facebook flow) before live publishing will succeed. See §5 checklist.
>
> The reference sections below document the mechanism and the exact CFN that was added (names
> in the examples use a generic prefix; the live names are the `ContentPublish*` ones above).
>
> **Why this doc exists:** Scheduled content publishing (brand app → "schedule post")
> reuses `pkg/delayed_sqs`, the same delayed-execution mechanism CrowdyChat already
> uses. The Go code is shared, but **the AWS resources and env vars it depends on are
> only provisioned for CrowdyChat (`serverless.cc.yml`), not for Trendly
> (`serverless.trendly.yml`).** This doc lists everything that must be added to make
> `delayed_sqs` work for Trendly.

---

## 1. How it works (mechanism)

```
delayedsqs.Send(message, delaySeconds)
        │  (states:StartExecution)
        ▼
Step Functions state machine  ──Wait $.delay_seconds──▶  sqs:sendMessage ──▶  SQS queue
                                                                                   │
                                                                                   ▼  (event source mapping)
                                                                          publish-consumer Lambda
                                                                                   │
                                                                                   ▼
                                                                       IG / FB publish + Firestore status update
```

- `delayedsqs.Send(message string, delayInSeconds int64)` starts a Step Functions
  execution. It reads two env vars:
  - `DELAY_STATE_FUNCTION` — the state machine ARN.
  - `SEND_MESSAGE_QUEUE_ARN` — the target SQS queue (the state machine's `sendMessage` target).
- The state machine `Wait`s for `delay_seconds`, then publishes the message body to the queue.
- The queue's event-source mapping triggers a **consumer Lambda** that does the actual work.
- `delayedsqs.StopExecutions(executionArn)` cancels a still-waiting execution
  (used for **cancel / reschedule** — store the returned `ExecutionArn` on the content doc).

Reference implementation already deployed: `serverless.cc.yml` (`CCDelayedSQS`,
`CCMessageQueue`, `CCStepFunctions`, `message_sqs` consumer).

---

## 2. What Trendly is missing

`serverless.trendly.yml` currently has **none** of:

| Piece | CrowdyChat name | Trendly — needs adding |
|---|---|---|
| Env: state machine ARN | `DELAY_STATE_FUNCTION` | ❌ add to `provider.environment` |
| Env: target queue | `SEND_MESSAGE_QUEUE_ARN` | ❌ add to `provider.environment` |
| SQS queue | `CCMessageQueue` | ❌ add `TrendlyScheduledPostQueue` |
| Step Functions state machine | `CCStepFunctions` (`CCDelayedSQS`) | ❌ add `TrendlyStepFunctions` |
| Step Functions IAM role | `CCStepFunctionsRole` | ❌ add `TrendlyStepFunctionsRole` |
| Lambda IAM: `states:StartExecution` / `StopExecution` / `sqs:sendMessage` | provider `iam.role.statements` | ❌ add |
| Consumer Lambda + SQS trigger | `message_sqs` | ❌ add `scheduled_publish_sqs` |

---

## 3. Additions to `serverless.trendly.yml`

### 3a. `provider.environment`

```yaml
provider:
  environment:
    # … existing vars …
    SEND_MESSAGE_QUEUE_ARN:
      Fn::GetAtt:
        - TrendlyScheduledPostQueue
        - QueueName
    DELAY_STATE_FUNCTION:
      Fn::GetAtt:
        - TrendlyStepFunctions
        - Arn
```

### 3b. `provider.iam.role.statements` (allow Lambdas to drive Step Functions + the queue)

```yaml
provider:
  iam:
    role:
      statements:
        # … existing statements …
        - Effect: "Allow"
          Action:
            - "states:StartExecution"
          Resource: { "Fn::GetAtt": ["TrendlyStepFunctions", "Arn"] }
        - Effect: "Allow"
          Action:
            - "states:StopExecution"
          Resource:
            Fn::Join:
              - ""
              - - "arn:aws:states:*:*:execution:"
                - { "Fn::GetAtt": ["TrendlyStepFunctions", "Name"] }
                - ":*"
        - Effect: "Allow"
          Action:
            - "sqs:SendMessage"
          Resource: { "Fn::GetAtt": ["TrendlyScheduledPostQueue", "Arn"] }
```

### 3c. `resources.Resources`

```yaml
resources:
  Resources:
    # … existing resources …

    TrendlyScheduledPostQueue:
      Type: AWS::SQS::Queue
      Properties:
        QueueName: TrendlyScheduledPostQueue
        VisibilityTimeout: 120          # ≥ consumer Lambda timeout

    TrendlyStepFunctions:
      Type: "AWS::StepFunctions::StateMachine"
      Properties:
        StateMachineName: TrendlyDelayedSQS
        RoleArn: !GetAtt TrendlyStepFunctionsRole.Arn
        DefinitionString: |
          {
            "StartAt": "Delay",
            "Comment": "Publish to SQS with delay",
            "States": {
              "Delay": {
                "Type": "Wait",
                "SecondsPath": "$.delay_seconds",
                "Next": "Send message to SQS"
              },
              "Send message to SQS": {
                "Type": "Task",
                "Resource": "arn:aws:states:::sqs:sendMessage",
                "Parameters": {
                    "QueueUrl.$": "$.topic",
                    "MessageBody.$": "$.message"
                },
                "End": true
              }
            }
          }

    TrendlyStepFunctionsRole:
      Type: "AWS::IAM::Role"
      Properties:
        Path: !Join ["", ["/", !Ref "AWS::StackName", "/"]]
        ManagedPolicyArns:
          - "arn:aws:iam::aws:policy/AWSStepFunctionsFullAccess"
        AssumeRolePolicyDocument:
          Version: "2012-10-17"
          Statement:
            - Sid: "AllowStepFunctionsServiceToAssumeRole"
              Effect: "Allow"
              Action:
                - "sts:AssumeRole"
              Principal:
                Service:
                  - !Sub "states.${AWS::Region}.amazonaws.com"
        Policies:
          - PolicyName: "PublishToSQSQueue"
            PolicyDocument:
              Version: "2012-10-17"
              Statement:
                - Effect: "Allow"
                  Action:
                    - "sqs:SendMessage"
                  Resource: "arn:aws:sqs:*"
```

### 3d. Consumer Lambda (`functions` section)

A new Lambda triggered by the queue, that reads the scheduled-post payload and publishes:

```yaml
functions:
  scheduled_publish_sqs:
    handler: bootstrap
    package:
      artifact: bin/scheduled_publish_sqs.zip
    events:
      - sqs:
          arn:
            Fn::GetAtt:
              - TrendlyScheduledPostQueue
              - Arn
          batchSize: 1
```

> Mirror how `message_sqs` is wired in `serverless.cc.yml`. The handler unmarshals the
> message body (e.g. `{ brandId, contentId, action: "PUBLISH" }`), loads the content
> doc, and calls the IG/FB publish path.

---

## 4. Message payload contract

`delayedsqs.Send` wraps the body; design the inner message as:

```json
{ "action": "PUBLISH", "brandId": "<id>", "contentId": "<id>" }
```

Keep it minimal (IDs only) and re-read the content doc in the consumer so edits made
after scheduling are respected at publish time.

---

## 5. Manual / one-time steps checklist

- [ ] Add the env vars (3a), IAM statements (3b), resources (3c), consumer Lambda (3d) to `serverless.trendly.yml`.
- [ ] Build + package the new `scheduled_publish_sqs` Lambda (Makefile / build script entry, like other functions).
- [ ] `sls deploy --stage dev --config serverless.trendly.yml` and confirm `TrendlyDelayedSQS` state machine + `TrendlyScheduledPostQueue` exist in the AWS console.
- [ ] Verify the deployed Lambdas have `DELAY_STATE_FUNCTION` and `SEND_MESSAGE_QUEUE_ARN` populated (Lambda → Configuration → Environment variables).
- [ ] Smoke test: call `delayedsqs.Send` with a 60s delay and confirm the consumer fires.
- [ ] Repeat for `--stage prod`.

---

## 6. Notes / decisions

- **Why `delayed_sqs` over EventBridge cron-scan:** chosen per review (02/06/2026) to
  reuse the proven CrowdyChat pattern; gives precise per-post scheduling and clean
  cancel/reschedule via `StopExecutions`, instead of a periodic Firestore scan.
- **Max delay:** Step Functions `Wait` supports up to 1 year — comfortably covers any
  realistic post scheduling horizon.
- **Cancel / reschedule:** persist the `ExecutionArn` returned by `Send` on the content
  doc; call `StopExecutions(arn)` to cancel, then `Send` again to reschedule.
- **Failure handling:** on publish failure the consumer sets `status = failed` +
  `errorMessage`; add an SQS redrive/DLQ if automatic retries are desired.
