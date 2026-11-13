// Package templates holds starting-point requirement lists for the role
// families Sinapis hires most often. HR edits them per vacancy — they are
// prompts for a good job description, not a final one.
//
// Note for HR: the scorer is told to ignore personal characteristics
// (religion, ethnicity, age, marital status, health). If Sinapis wants to
// assess mission or values alignment, do it through an application question
// and the interview, and confirm the approach with whoever advises Sinapis on
// employment law.
package templates

// RoleTemplate prefills a new job.
type RoleTemplate struct {
	Key                string
	Label              string
	Summary            string
	RequiredExperience []string
	RequiredEducation  []string
	RequiredDuties     []string
	NiceExperience     []string
	NiceEducation      []string
	NiceDuties         []string
	Questions          []string
}

// All returns the templates in display order.
func All() []RoleTemplate { return all }

// Get returns a template by key.
func Get(key string) (RoleTemplate, bool) {
	for _, t := range all {
		if t.Key == key {
			return t, true
		}
	}
	return RoleTemplate{}, false
}

var commonQuestions = []string{
	"Why do you want to work with Sinapis, and what draws you to supporting entrepreneurs?",
	"Describe a result you achieved in a similar role. What was your part, and how did you measure it?",
}

var all = []RoleTemplate{
	{
		Key: "general", Label: "Blank (start from scratch)",
	},
	{
		Key: "programme", Label: "Programme / Business Development Services",
		Summary: "Deliver Sinapis entrepreneurship programmes — training, mentorship and business support for small and growing businesses.",
		RequiredExperience: []string{
			"3+ years delivering entrepreneurship, enterprise development or SME support programmes",
			"Experience training or coaching business owners in groups and one-to-one",
			"Experience coordinating programme logistics, cohorts and partners",
		},
		RequiredEducation: []string{"Bachelor's degree in Business, Economics, Development Studies or a related field"},
		RequiredDuties: []string{
			"Deliver business training sessions and facilitate workshops",
			"Recruit, onboard and support programme participants",
			"Track participant progress and programme outcomes",
			"Manage relationships with mentors, facilitators and partner organisations",
			"Prepare programme reports for management and funders",
		},
		NiceExperience: []string{"Experience with accelerator or investment-readiness programmes", "Experience working with women- or youth-led businesses", "Experience in more than one East African country"},
		NiceEducation:  []string{"Postgraduate qualification or certification in business advisory, coaching or entrepreneurship"},
		NiceDuties:     []string{"Design or adapt training curricula", "Use a CRM or data system to manage participant records"},
		Questions:      commonQuestions,
	},
	{
		Key: "finance", Label: "Finance & Accounting",
		Summary: "Manage Sinapis's financial operations, donor fund accountability and compliance.",
		RequiredExperience: []string{
			"3+ years in accounting or finance, preferably in a non-profit or donor-funded organisation",
			"Experience preparing financial statements, budgets and management accounts",
			"Experience with donor grant financial reporting and compliance",
		},
		RequiredEducation: []string{"Bachelor's degree in Finance, Accounting or Commerce", "CPA (K) or equivalent professional qualification (CPA, ACCA)"},
		RequiredDuties: []string{
			"Maintain books of accounts and perform monthly reconciliations",
			"Prepare budgets, forecasts and variance reports",
			"Prepare donor financial reports to agreed formats and deadlines",
			"Ensure statutory compliance (tax, payroll deductions, filings)",
			"Support internal and external audits",
		},
		NiceExperience: []string{"Experience with QuickBooks, Sage or a similar accounting system", "Multi-currency or multi-country (Kenya, Uganda, Rwanda) accounting"},
		NiceDuties:     []string{"Train programme staff on financial procedures", "Improve financial controls and policies"},
		Questions:      commonQuestions,
	},
	{
		Key: "mel", Label: "Monitoring, Evaluation & Learning / Data",
		Summary: "Measure and communicate the impact of Sinapis programmes through sound data systems and analysis.",
		RequiredExperience: []string{
			"3+ years in monitoring and evaluation, impact measurement or data analysis",
			"Experience designing surveys and data collection tools",
			"Experience analysing quantitative and qualitative data",
		},
		RequiredEducation: []string{"Bachelor's degree in Statistics, Economics, Social Sciences, Data Science or a related field"},
		RequiredDuties: []string{
			"Design and maintain M&E frameworks, indicators and data collection tools",
			"Clean, analyse and report on programme and impact data",
			"Produce dashboards and reports for management, funders and partners",
			"Train staff on data collection and data quality",
		},
		NiceExperience: []string{"Experience with Salesforce, HubSpot or similar CRMs", "Experience with Python, R, Stata or SQL", "Experience with impact frameworks (IRIS+, IMP)"},
		NiceEducation:  []string{"Postgraduate qualification in M&E, statistics or research methods"},
		NiceDuties:     []string{"Lead evaluations or learning reviews", "Communicate impact stories for marketing and fundraising"},
		Questions:      commonQuestions,
	},
	{
		Key: "operations", Label: "Operations & Administration",
		Summary: "Keep Sinapis running smoothly — office, procurement, logistics, HR administration and events.",
		RequiredExperience: []string{
			"2+ years in office administration, operations or logistics",
			"Experience managing procurement, vendors and contracts",
		},
		RequiredEducation: []string{"Diploma or Bachelor's degree in Business Administration or a related field"},
		RequiredDuties: []string{
			"Coordinate office operations, assets and supplies",
			"Run procurement processes and manage vendor relationships",
			"Organise travel, events and programme logistics",
			"Maintain administrative and HR records",
		},
		NiceExperience: []string{"Experience in a non-profit or donor-funded organisation", "Event management experience"},
		NiceDuties:     []string{"Support HR processes (onboarding, leave, payroll inputs)"},
		Questions:      commonQuestions,
	},
	{
		Key: "communications", Label: "Communications & Marketing",
		Summary: "Tell the story of Sinapis entrepreneurs and grow our reach with partners, funders and applicants.",
		RequiredExperience: []string{
			"3+ years in communications, marketing or public relations",
			"Experience writing for digital channels, reports and media",
			"Experience managing social media channels and campaigns",
		},
		RequiredEducation: []string{"Bachelor's degree in Communications, Journalism, Marketing or a related field"},
		RequiredDuties: []string{
			"Produce content (stories, posts, newsletters, case studies)",
			"Manage social media and website content",
			"Plan and run campaigns for programme recruitment and events",
			"Track and report on communications performance",
		},
		NiceExperience: []string{"Photography, video or design skills", "Experience with HubSpot or email marketing tools"},
		NiceDuties:     []string{"Coordinate media relations", "Support fundraising communications"},
		Questions:      commonQuestions,
	},
	{
		Key: "technology", Label: "Technology / Systems",
		Summary: "Build and support the systems Sinapis staff and entrepreneurs rely on.",
		RequiredExperience: []string{
			"3+ years in software development, systems administration or IT support",
			"Experience building or maintaining web applications or business systems",
		},
		RequiredEducation: []string{"Bachelor's degree in Computer Science, IT or a related field, or equivalent experience"},
		RequiredDuties: []string{
			"Develop, configure and maintain internal systems",
			"Integrate systems and manage data flows between them",
			"Support users and document processes",
			"Keep systems secure and data protected",
		},
		NiceExperience: []string{"Go, Python or JavaScript", "Salesforce, HubSpot or Google Workspace administration", "Experience with cloud hosting (GCP, AWS, Render)"},
		NiceEducation:  []string{"Relevant certifications (e.g. Salesforce Administrator, cloud certifications)"},
		NiceDuties:     []string{"Train staff on tools", "Contribute to data protection compliance"},
		Questions:      commonQuestions,
	},
}
