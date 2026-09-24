package domain

// ChatProviderName identifies an external chat or messaging network.
type ChatProviderName string

const (
	ChatProviderSlack   ChatProviderName = "slack"
	ChatProviderTeams   ChatProviderName = "teams"
	ChatProviderDiscord ChatProviderName = "discord"
	ChatProviderIRC     ChatProviderName = "irc"
	ChatProviderMatrix  ChatProviderName = "matrix"
)

// IsValid validates whether a chat provider name is recognized.
func (c ChatProviderName) IsValid() bool {
	switch c {
	case ChatProviderSlack, ChatProviderTeams, ChatProviderDiscord, ChatProviderIRC, ChatProviderMatrix:
		return true
	default:
		return false
	}
}
