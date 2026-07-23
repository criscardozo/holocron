package templates

// SystemView is the formatted view model for the system widget. Values are
// pre-rendered strings ("—" when unavailable) so the template stays free of
// formatting logic.
type SystemView struct {
	CPU    string
	RAM    string
	Temp   string
	Uptime string
	Load   string
}
