package label

const (
	AlertCenterModuleName = "告警中心"

	// Alert routes (告警路由策略)
	AlertRouteList   = "alertRouteList"
	AlertRouteCreate = "alertRouteCreate"
	AlertRouteUpdate = "alertRouteUpdate"
	AlertRouteDelete = "alertRouteDelete"

	// Notification templates (通知消息模板)
	TemplateList   = "templateList"
	TemplateCreate = "templateCreate"
	TemplateUpdate = "templateUpdate"
	TemplateDelete = "templateDelete"

	// Aggregation policies (告警聚合策略)
	AggregationList   = "aggregationList"
	AggregationCreate = "aggregationCreate"
	AggregationUpdate = "aggregationUpdate"
	AggregationDelete = "aggregationDelete"

	// Silence policies (告警静默策略)
	SilenceList   = "silenceList"
	SilenceCreate = "silenceCreate"
	SilenceDelete = "silenceDelete"

	// Notification policies (通知策略)
	PolicyList   = "policyList"
	PolicyCreate = "policyCreate"
	PolicyUpdate = "policyUpdate"
	PolicyDelete = "policyDelete"

	// Notification contacts (联系人)
	ContactList   = "contactList"
	ContactCreate = "contactCreate"
	ContactUpdate = "contactUpdate"
	ContactDelete = "contactDelete"

	// Notification contact groups (联系人组)
	ContactGroupList   = "contactGroupList"
	ContactGroupCreate = "contactGroupCreate"
	ContactGroupUpdate = "contactGroupUpdate"
	ContactGroupDelete = "contactGroupDelete"

	// Inhibit rules (告警抑制策略)
	InhibitList   = "inhibitList"
	InhibitCreate = "inhibitCreate"
	InhibitUpdate = "inhibitUpdate"
	InhibitDelete = "inhibitDelete"

	// Escalation policies (告警升级策略)
	EscalationList   = "escalationList"
	EscalationCreate = "escalationCreate"
	EscalationUpdate = "escalationUpdate"
	EscalationDelete = "escalationDelete"

	// Webhook sources (通用 Webhook 可信源 / RFC 9421 keypair)
	WebhookSourceList   = "webhookSourceList"
	WebhookSourceCreate = "webhookSourceCreate"
	WebhookSourceUpdate = "webhookSourceUpdate"
	WebhookSourceDelete = "webhookSourceDelete"
	WebhookSourceRotate = "webhookSourceRotate"
)
