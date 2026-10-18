package models

// GlobalGameCode is the combined board. nobody submits to it directly, the
// server adds every score to it as well as to the game's own board.
const GlobalGameCode = "global"

// PlayableGameCodes are the games a score can be submitted to. same list as
// the games seeded in migrations/0001_init.sql, so add new games to both.
var PlayableGameCodes = []string{"speed-run", "trivia", "puzzle"}

// IsPlayableGame is true if a score can be submitted to this code.
func IsPlayableGame(code string) bool {
	for _, c := range PlayableGameCodes {
		if c == code {
			return true
		}
	}
	return false
}

// IsKnownGame is true for a playable game or the global board. read
// endpoints use it so a request can't look up random Redis keys.
func IsKnownGame(code string) bool {
	return code == GlobalGameCode || IsPlayableGame(code)
}

// AllBoardCodes lists every board, global first.
func AllBoardCodes() []string {
	return append([]string{GlobalGameCode}, PlayableGameCodes...)
}
