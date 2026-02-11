# Postman Collection Generation Context

This document provides context for generating Postman Collection v2.1 JSON files from the `backend-sls` codebase. Use this as a reference whenever the user asks to create a Postman collection for any set of APIs.

## Output Format

- **Always generate Postman Collection v2.1 JSON** (schema: `https://schema.getpostman.com/json/collection/v2.1.0/collection.json`).
- Save the output file to: `postman/<name>_collection.json`.
- One file per import — the user should be able to drag it into Postman and have everything ready.

## User Preferences

- **Base URL**: Use `{{baseUrl}}` as the environment variable. The user already has this configured in their Postman environment — do NOT include a separate environment JSON file.
- **Authentication**: Do NOT configure any auth at the collection or folder level. The user has auth (Bearer token / session) configured at their base Postman collection level, and it inherits down.
- **Collection name**: Use `"Trendly Backend"` as the top-level collection name (so it merges with their existing collection on re-import).

## Folder Structure Convention

- Each set of APIs should be placed inside a **top-level named folder** (e.g., "Monetization", "Onboarding", etc.).
- Inside that, **sub-folders should be grouped by actor + domain** (e.g., "Brand - Orders", "Influencer - Shipment", "Webhooks").
- Use your best judgment to segregate based on the route groups and middleware groupings visible in the Go handler files.
- Each sub-folder should have a short `description` summarizing its purpose.

## How to Extract API Information

### 1. Identify the Lambda Handler Entry Point
Look in `/functions/<function_name>/main.go`. This file defines:
- The base route group (e.g., `/monetize`)
- Sub-groups with middlewares (e.g., `/brands`, `/influencers`, `/webhooks`)
- All route registrations with HTTP methods and handler function references

### 2. Read the Handler Implementations
Handler functions live in `/internal/trendlyapis/<domain>/` (e.g., `/internal/trendlyapis/monetize/`). From each handler, extract:
- **Request body struct**: Look for `c.ShouldBindJSON(&req)` or `c.ShouldBind(&req)` — the struct fields with `json:"..."` tags define the request body.
- **Path parameters**: Look for `c.Param("paramName")` — these become `:paramName` in the URL.
- **Query parameters**: Look for `c.Query("paramName")` — these become query params.
- **Required vs Optional**: Fields with `binding:"required"` are required. Fields without are optional.
- **Response shape**: Look for `c.JSON(http.StatusOK, gin.H{...})` at the end of the handler to understand the success response.

### 3. Check for Nested Structs
If the request body references types from other packages (e.g., `payments.AddressReq`, `payments.BankReq`), follow the import to get the full field definitions. These are typically in `/pkg/<package>/`.

## Per-Request JSON Structure

For each request in the collection:

```json
{
    "name": "Human-readable name (from the code comment or route purpose)",
    "request": {
        "method": "POST|GET|PUT|DELETE|ANY",
        "header": [
            {
                "key": "Content-Type",
                "value": "application/json"
            }
        ],
        "body": {
            "mode": "raw",
            "raw": "<pre-filled JSON with realistic sample data>",
            "options": {
                "raw": {
                    "language": "json"
                }
            }
        },
        "url": {
            "raw": "{{baseUrl}}/full/path/:param",
            "host": ["{{baseUrl}}"],
            "path": ["full", "path", ":param"],
            "variable": [
                {
                    "key": "param",
                    "value": "",
                    "description": "Description of the path param"
                }
            ]
        },
        "description": "Detailed description covering:\n- What the endpoint does\n- Required fields with types\n- Optional fields with types\n- Side effects (notifications, status changes)\n- Any enum/scenario values"
    }
}
```

### Sample Data Guidelines
- Use realistic Indian sample data where relevant (e.g., PAN: `ABCDE1234F`, IFSC: `HDFC0001234`, cities like Mumbai/Bangalore).
- Use epoch milliseconds (int64) for date fields (e.g., `1740000000000`).
- Use placeholder URLs for file/image fields (e.g., `https://example.com/proof.jpg`).
- For endpoints with no request body, use `"{}"` as the raw body.
- GET requests should not have a body section.

### Description Guidelines
Each request description should include:
- One-liner of what the API does.
- **Required fields** listed with types and explanation.
- **Optional fields** listed with types.
- Any **enum values** explained (e.g., postingScenario: 1 = Influencer posts, 2 = Collab, 3 = Brand only).
- Notable **side effects** (e.g., "Sets contract status to 4", "Sends email to brand members").

## Route Pattern Reference

The codebase uses Gin router groups. The typical pattern is:

```go
// Entry point: functions/<name>/main.go
handler := apihandler.GinEngine.Group("/basePath", middlewares.ValidateSessionMiddleware())
subGroup := handler.Group("/subPath", middlewares.TrendlyMiddleware("collection"))
subGroup.POST("/action", handlerPackage.HandlerFunc)
```

This translates to: `POST {{baseUrl}}/basePath/subPath/action`

## Middleware Hints
- `ValidateSessionMiddleware()` — Session/auth required (user already handles this in Postman).
- `TrendlyMiddleware("brands")` — Brand-specific middleware (sets brand context).
- `TrendlyMiddleware("users")` — User/Influencer-specific middleware (sets user context).
- No middleware on webhooks group — these are called by external services (Razorpay).

## Existing Collections
- `postman/monetization_collection.json` — Monetization APIs (Brand orders, shipment, deliverables, posting; Influencer account, shipment, deliverables, posting; Webhooks).
