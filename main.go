package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/notnil/chess"
)

type gameState struct {
	Type  string
	State struct {
		Moves  string
		Status string
	}

	White struct {
		Id string
	}
	Black struct {
		Id string
	}

	Moves string
}

type settings struct {
	Token  string
	GameId string
	User   string

	Client *http.Client
}

var algebraicNotation = chess.AlgebraicNotation{}
var longAlgebraicNotation = chess.LongAlgebraicNotation{}

func startStream(s settings) (*http.Response, error) {
	url := fmt.Sprintf("https://lichess.org/api/board/game/stream/%s", s.GameId)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := doLichessRequest(s, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Couldn't start streaming")
	}

	return resp, nil
}

func sendMove(s settings, position *chess.Position, move *chess.Move) error {
	url := fmt.Sprintf("https://lichess.org/api/board/game/%s/move/%s", s.GameId, longAlgebraicNotation.Encode(position, move))

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := doLichessRequest(s, req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("Couldn't send the move %s", move)
	}

	return nil
}

func resign(s settings) error {
	url := fmt.Sprintf("https://lichess.org/api/board/game/%s/resign", s.GameId)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := doLichessRequest(s, req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("Couldn't send resignation")
	}

	return nil
}

func doLichessRequest(s settings, req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", s.Token))

	return s.Client.Do(req)
}

func main() {
	var s settings

	flag.StringVar(&s.Token, "token", "", "lichess auth token (with board:play scope)")
	flag.StringVar(&s.GameId, "gameid", "", "lichess game id")
	flag.StringVar(&s.User, "user", "", "lichess user id")
	flag.Parse()

	if s.Token == "" {
		log.Fatal("Missing lichess auth token")
	}

	if s.GameId == "" {
		log.Fatal("Missing lichess game id")
	}

	if s.User == "" {
		log.Fatal("Missing lichess user id")
	}

	s.User = strings.ToLower(s.User)
	s.Client = &http.Client{}

	resp, err := startStream(s)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	playingWithWhite := true
	localGame := chess.NewGame(chess.UseNotation(algebraicNotation))
	remoteGame := chess.NewGame(chess.UseNotation(longAlgebraicNotation))

	completer := func(d prompt.Document) []prompt.Suggest {
		s := []prompt.Suggest{
			{Text: "resign", Description: "resign the game"},
			{Text: "exit", Description: "exit the cli"},
		}
		for _, m := range localGame.ValidMoves() {
			s = append(s, prompt.Suggest{Text: algebraicNotation.Encode(localGame.Position(), m)})
		}

		return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
	}

	dec := json.NewDecoder(resp.Body)

	for dec.More() {
		var state gameState
		err := dec.Decode(&state)
		if err != nil {
			log.Fatal(err)
		}

		if state.Type == "gameFull" {
			if state.State.Status != "started" {
				log.Fatal("game is not active")
			}

			moves := strings.Split(state.State.Moves, " ")
			for _, m := range moves {
				if err := remoteGame.MoveStr(m); err != nil {
					log.Fatal(err)
				}
			}

			for _, m := range remoteGame.Moves() {
				localGame.Move(m)
			}

			if state.White.Id == s.User {
				playingWithWhite = true
			} else if state.Black.Id == s.User {
				playingWithWhite = false
			} else {
				log.Fatal("user does not match with any of the sides on the game state from lichess")
			}

			isWhiteTurn := len(moves)%2 == 0
			if (playingWithWhite && isWhiteTurn) || (!playingWithWhite && !isWhiteTurn) {
				for {
					t := prompt.Input("> ", completer)

					switch t {
					case "exit":
						os.Exit(0)
					case "resign":
						resign(s)
						os.Exit(0)
					}

					err := localGame.MoveStr(t)

					if err != nil {
						fmt.Println("Invalid move", err)
					} else {
						err := sendMove(s, localGame.Position(), localGame.Moves()[len(localGame.Moves())-1])
						if err != nil {
							log.Fatal(err)
						}
						break
					}
				}
			}
		} else if state.Type == "gameState" {
			moves := strings.Split(state.Moves, " ")
			for i := len(remoteGame.Moves()); i < len(moves); i++ {
				m := moves[i]
				if err := remoteGame.MoveStr(m); err != nil {
					log.Fatal(err)
				}

				if i >= len(localGame.Moves()) {
					localGame.Move(remoteGame.Moves()[i])
				}
			}

			isWhiteTurn := len(moves)%2 == 0
			if (playingWithWhite && isWhiteTurn) || (!playingWithWhite && !isWhiteTurn) {
				for {
					t := prompt.Input("> ", completer)

					switch t {
					case "exit":
						os.Exit(0)
					case "resign":
						resign(s)
						os.Exit(0)
					}

					err := localGame.MoveStr(t)

					if err != nil {
						fmt.Println("Invalid move", err)
					} else {
						err := sendMove(s, localGame.Position(), localGame.Moves()[len(localGame.Moves())-1])
						if err != nil {
							log.Fatal(err)
						}
						break
					}
				}
			}
		}

	}
}
