# Antigravity Project Context for `backend-sls`

## Project Overview
This repository (`backend-sls`) is a monorepo-style serverless backend utilizing Go (v1.23) and AWS Lambda. It powers two distinct applications:
1. **Trendly**: A platform connecting brands, influencers, and collaborations (features include matchmaking, discovery, monetization/razorpay, social media integration).
2. **Crowdy Chat (CC)**: A chat and messaging application (features include websockets, SQS messaging, StepFunctions).

## Technology Stack
- **Languages**: Go 1.23 (primary), Node.js (minor, for specific handlers), Shell scripts.
- **Frameworks**: 
  - **Serverless Framework (v3)**: Infrastructure as Code (`serverless.trendly.yml`, `serverless.cc.yml`).
  - **Gin**: Go HTTP web framework (used within Lambda handlers via `aws-lambda-go-api-proxy` or similar patterns).
- **Cloud Provider**: AWS (Lambda, S3, SQS, Step Functions, CloudFront).
- **Database**:
  - **Firestore**: Primary database for application models (`trendlymodels`, `models`).
  - **DynamoDB**: Potentially used (based on `dynamo_data` folder), but Firestore is the active choice for core models.

## Key Directories Structure
- **`/cmd`**: Standard Go entry points.
- **`/internal`**: Shared business logic and domain code.
  - `trendlyapis`: API logic for Trendly.
  - `ccapis`: API logic for Crowdy Chat.
  - `models`: Shared data models.
  - `middlewares`: HTTP middlewares.
  - `constants`: Shared constants.
- **`/functions`**: Lambda function handlers (Go `main` packages).
- **`/pkg`**: Shared utility modules and adapters.
- **`/scripts`**: Standalone scripts or cron-job handlers.
- **`/configs`**: Configuration files.

## Data Models (`/internal/models`)
The project uses **Firestore** as the primary data store for these models.

### `trendlymodels` (Core Trendly Entities)
- **Brand** (`brand.go`): Represents a brand, including profile, billing, preferences, and connected influencers.
- **Contract** (`contract.go`): Manages contracts between brands and influencers.
- **Collaboration** (`collaboration.go`): Manages collaboration workflows.
- **User** (`user.go`): User management.
- **Notification** (`notification.go`): System notifications.
- **Invite** (`invite.go`, `influencer_invite.go`): Invitation systems.
- **Social** (`social.go`): Social media linking.
- **Other**: `manager.go`, `brand_member.go`.

### `trendlybq` (BigQuery Related)
- **Influencers** (`influencers.go`)
- **Socials** (`socials.go`)

### Shared/Legacy Models (Root of `/internal/models`)
- **Campaign** (`campaign.go`): Campaign management (uses Firestore).
- **Conversation** (`conversation.go`)
- **Lead** (`lead.go`, `lead_stage.go`)
- **Collectibles** (`collectibles.go`)
- **Source** (`source.go`)

## Shared Packages (`/pkg`)
These modules provide reusable utilities and external service adapters:
- **`firebase`**: Firebase/Firestore integration helpers.
- **`hubspot`**: HubSpot CRM integration.
- **`instagram`**: Instagram Graph API integration.
- **`streamchat`**: Integration with Stream Chat SDK.
- **`payments`**: Payment processing utilities (likely Razorpay/Stripe).
- **`myopenai`**: OpenAI API wrappers/utilities.
- **`messenger`**: Facebook/Meta messenger integration.
- **`myemail`**: Email sending utilities (SendGrid).
- **`mytime`**: Time handling utilities.
- **`myutil`**: Generic utility functions.
- **`delayed_sqs`**: Handling delayed messages (likely via Step Functions).
- **`sqs_handler`**: SQS message processing helpers.
- **`dynamodb_handler`**: DynamoDB helpers (legacy or specific use).
- **`ws_handler`**: WebSocket handling logic.
- **`api_handler`**: Standardized API response/request handling.
- **`middlewares`**: Shared HTTP middlewares.

## Key Configuration (Serverless)
### Trendly (`serverless.trendly.yml`)
- **Service Name**: `trendly-be`
- **Domain**: `be.trendly.now`
- **Functions**: `trendly_discovery`, `trendly_contracts`, `trendly_matchmaking`, `trendly_razorpay_apis`, `insta_apis`, `t_websocket`.

### Crowdy Chat (`serverless.cc.yml`)
- **Service Name**: `crowdy-chat-be`
- **Domain**: `be.crowdy.chat`
- **Functions**: `cc_websocket`, `cc_backend`, `cc_message_sqs`.

## Coding Conventions
- **Handlers**: Go files used as handlers usually have a `main` package and `main` function.
- **Build Tags**: `lambda.norpc` is used (e.g., `go build -tags lambda.norpc`).
- **JSON & Firestore**: Structs have both `json:"..."` and `firestore:"..."` tags.
- **Error Handling**: Explicit error checking; recurring pattern of returning `c.JSON(http.StatusBadRequest, ...)` on errors.
- **Dependency Injection**: Services often initialize their own clients or use global clients from `pkg`.
