# Trendly Email System Context

This document provides context for creating, managing, and sending emails within the `backend-sls` project. Use this as a reference whenever email-related tasks are requested.

## Directory Structure
- **Templates**: `/Users/rsinha/iDiv/backend-sls/templates/` (HTML files)
- **Template Paths**: `templates/init.go` (Constants for finding templates)
- **Email Subjects**: `templates/subject.go` (Constants for subject lines)
- **Sending Logic**: `pkg/myemail/` (SendGrid integration and helper functions)

## Email Template Format
Templates are standard HTML files using Go `html/template` syntax.
### Consistency Rules:
1. **Header Comment**: Every template MUST start with a commented block listing all dynamic variables used in the file.
   ```html
   <!-- 
     Dynamic Variables:
       {{.RecipientName}} => Name of the recipient
       {{.SubjectName}}   => Name of the related entity (e.g., brand, influencer)
       {{.Link}}          => Actionable link
   -->
   ```
2. **Dynamic Variables**: Use `{{.VariableName}}` throughout the HTML body.
3. **Consistent Styling**: Reuse styles from existing templates (e.g., `influencer_accepted.html`) to ensure brand consistency.

## Sending Emails from Code
Emails are triggered using the `myemail` package. The most common pattern involves:
1. Preparing a `map[string]interface{}` with dynamic data.
2. Calling `myemail.SendCustomHTMLEmail` or `myemail.SendCustomHTMLEmailToMultipleRecipients`.

### Example Pattern:
```go
data := map[string]interface{}{
    "BrandMemberName": brand.Name,
    "InfluencerName":  user.Name,
    "CollabTitle":     collab.Name,
    "PokeTime":        mytime.FormatPrettyIST(time.Now()),
    "EndLink":         fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
}

// Single recipient
err = myemail.SendCustomHTMLEmail(recipientEmail, templates.CollaborationEndNudged, templates.SubjectNudgeToEndContract, data)

// Multiple recipients
err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationEndNudged, templates.SubjectNudgeToEndContract, data)
```

## Adding a New Email
To add a new email flow, follow these steps:
1. **Create HTML Template**: Add a new `.html` file in the `templates/` folder with the required variable comments.
2. **Register Template Path**: Add a constant in `templates/init.go`.
   ```go
   MyNewTemplate myemail.TemplatePath = "templates/my_new_template.html"
   ```
3. **Define Subject**: Add a constant in `templates/subject.go`.
   ```go
   SubjectMyNewTemplate = "Your new update from Trendly!"
   ```
4. **Invoke in Handler**: Use the `myemail` helper functions in the relevant API handler.

## Available Sending Functions (`pkg/myemail/main.go`)
- `SendCustomHTMLEmail(toEmail string, templatePath TemplatePath, subject string, data map[string]interface{}) error`
- `SendCustomHTMLEmailToMultipleRecipients(toEmails []string, templatePath TemplatePath, subject string, data map[string]interface{}) error`
- `SendEmailUsingTemplate(toEmail, templateID string, dynamicData map[string]interface{}) error` (Used for SendGrid Dynamic Templates, less common for custom HTML).
