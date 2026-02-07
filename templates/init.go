package templates

import "github.com/idivarts/backend-sls/pkg/myemail"

// /Users/rsinha/iDiv/backend-sls/
const (
	ApplicationAccepted               myemail.TemplatePath = "templates/application_accepted.html"
	ApplicationRejected               myemail.TemplatePath = "templates/application_rejected.html"
	ApplicationSent                   myemail.TemplatePath = "templates/application_sent.html"
	EmailVerification                 myemail.TemplatePath = "templates/email_verification.html"
	CollaborationEndedInfluencer      myemail.TemplatePath = "templates/end_contract_influencer.html"
	CollaborationEndedBrand           myemail.TemplatePath = "templates/end_contract_brand.html"
	InfluencerInvitedToCollab         myemail.TemplatePath = "templates/invitation_sent.html"
	CollaborationQuotationResubmitted myemail.TemplatePath = "templates/new_quotation.html"
	PasswordReset                     myemail.TemplatePath = "templates/password_reset.html"
	PasswordChanged                   myemail.TemplatePath = "templates/password_changed.html"
	CollaborationEndNudged            myemail.TemplatePath = "templates/poke_end_contract.html"
	CollaborationStartRequested       myemail.TemplatePath = "templates/poke_start_collaboration.html"
	CollaborationRatedByInfluencer    myemail.TemplatePath = "templates/rating_received.html"
	CollaborationQuotationRequested   myemail.TemplatePath = "templates/revise_quotation.html"
	CollaborationStarted              myemail.TemplatePath = "templates/start_collaboration.html"
	InfluencerJoined                  myemail.TemplatePath = "templates/welcome_influencer.html"
	BrandCreated                      myemail.TemplatePath = "templates/welcome_brand.html"
	MessageReminder                   myemail.TemplatePath = "templates/message_reminder.html"
	BrandEmailInvite                  myemail.TemplatePath = "templates/brand_email_invite.html"
	InfluencerInvite                  myemail.TemplatePath = "templates/influencer_invite.html"
	InfluencerInviteAccepted          myemail.TemplatePath = "templates/influencer_accepted.html"
	InfluencerInviteRejected          myemail.TemplatePath = "templates/influencer_rejected.html"
	CollaborationTakedown             myemail.TemplatePath = "templates/collaboration_takedown.html"
	PaymentOrderCreated               myemail.TemplatePath = "templates/payment_order_created.html"
)
