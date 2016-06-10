package irc

const (
	// Numeric reply codes
	RPL_WELCOME          = "001"
	RPL_YOURHOST         = "002"
	RPL_CREATED          = "003"
	RPL_MYINFO           = "004"
	RPL_MOTD             = "372"
	RPL_MOTDSTART        = "375"
	RPL_ENDOFMOTD        = "376"
	ERR_UNKNOWNCOMMAND   = "421"
	ERR_NONICKNAMEGIVEN  = "431"
	ERR_ERRONEUSNICKNAME = "432"
	ERR_NICKNAMEINUSE    = "433"
	ERR_NICKCOLLISION    = "436"
	ERR_NEEDMOREPARAMS   = "451"
)
