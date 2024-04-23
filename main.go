package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ChessGame struct {
	//made an unsigned int because never needing this value to be negative.
	//Also there is currently a process running somewhere that is constantly generating new IDs,
	//so changing from int to uint64 is a stopgap measure to prevent the need to constantly clear the
	//database in order to allow games to be created at any moment
	ID          uint64 `bson: "id"`
	PGN         string `bson: "pgn"`
	CurrentTurn string `bson: "currentTurn"`
	IsGameLive  bool   `bson: "isGameLive"`
}

type EndGameResponse struct {
	Success        bool   `json: "success"`
	IsQuit         bool   `json:"isQuit "`
	EndingPlayer   string `json: "endingPlayer"`
	FailureMessage string `json: "failureMessage"`
}

var DB_USER string = "adamrhayes2"
var DB_PASS string = "QtuyfScPfmF7haXg"

var client *mongo.Client
var database *mongo.Database
var collection *mongo.Collection

//sourcing file structure from https://tutorialedge.net/golang/creating-restful-api-with-golang/

func main() {
	router := gin.Default()
	router.Use(corsMiddleware())
	//all endpoints tested using postman before attempmting programmatic connection with application front end

	//todo: write openAPI specification for documentation before submitting
	router.GET("/new", createNewGame)
	router.POST("/live/:id", enterPlayerTurn)
	router.DELETE("/end/:id/:endingPlayer", endGame)
	router.DELETE("/quit/:id/:endingPlayer", quitGame)
	router.GET("/live/:id", retrieveGameState)
	router.GET("/", ping)

	router.Run()

}

func ping(c *gin.Context) {
	c.JSON(http.StatusOK, "ping")
}

func connectToMongoDB() *mongo.Client {
	//code sample grabbed from MongoDB setup page

	log.Println("attempting mongoDB connection")
	// Use the SetServerAPIOptions() method to set the version of the Stable API on the client
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI("mongodb+srv://" + DB_USER + ":" + DB_PASS + "@cluster0.0mr298k.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0").SetServerAPIOptions(serverAPI)
	// Create a new client and connect to the server
	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		client.Disconnect(context.TODO())
		panic(err)
	}
	database = client.Database("chess")
	collection = database.Collection("savedGames")

	// Send a ping to confirm a successful connection
	if err := client.Database("admin").RunCommand(context.TODO(), bson.D{{"ping", 1}}).Err(); err != nil {
		client.Disconnect(context.TODO())
		panic(err)
	}
	log.Println("mongoDB connection created")
	return client
}

// creates new record in storage,
// returns id
func createNewGame(c *gin.Context) {

	client = connectToMongoDB()
	newGame := new(ChessGame)
	newId := generateNewGameRecord()
	if newId > 0 {
		newGame.ID = newId
		newGame.PGN = ""
		newGame.IsGameLive = true
		result, err := collection.InsertOne(context.TODO(), newGame)
		log.Println("result")
		client.Disconnect(context.TODO())
		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			panic(err)
		} else {
			if result != nil {
				log.Println("Game Started")
				log.Println(newGame)
				c.JSON(http.StatusOK, newGame)
			}
		}

	} else {
		client.Disconnect(context.TODO())
		c.JSON(http.StatusInternalServerError, "unable to generate incremental ID for new game")
	}
}

func generateNewGameRecord() uint64 {

	//  mongoDB connection when opened in Main kept getting cut off by the time this code was reached,
	// need to either open a new connection everytime (inefficient but simple)
	// or figure out a way to keep connection open

	sort := options.FindOne().SetSort(bson.D{{Key: "id", Value: -1}})
	filter := bson.D{}

	singleResult := collection.FindOne(context.TODO(), filter, sort)

	//todo DEBUG: this method is now only churning out the number 10
	var newId uint64
	if singleResult.Err() != nil {
		log.Println("No Results Found, attempting error comparison")
		//consider consolidating response code handling for SingleResult object types (shared between newGame and getGame)
		if errors.Is(singleResult.Err(), mongo.ErrNoDocuments) {
			log.Println("Collection was empty, starting IDs at initial value (1)")
			newId = 1
		} else {
			log.Println(singleResult.Err())
			newId = 0
		}
	} else {
		log.Println(singleResult)
		var BSONData *ChessGame
		BSONData = new(ChessGame)
		err := singleResult.Decode(&BSONData)
		if err != nil {
			log.Println(err)
			return 0
		}

		log.Println("id to increment: " + strconv.FormatUint(BSONData.ID, 10))
		newId = BSONData.ID
		newId++
		log.Println("value after increment " + strconv.FormatUint(BSONData.ID, 10))

	}
	return newId
}

// takes an id, and the pgn of the board resulting from their turn,
// and saves the record

func enterPlayerTurn(c *gin.Context) {
	log.Println("Attempting to save turn in database")
	log.Println(c.Request.Body)
	var turnToEnter ChessGame
	if err := c.BindJSON(&turnToEnter); err != nil {
		log.Println("Unable to convert input into struct")
		c.JSON(http.StatusBadRequest, err)
		panic(err)
	}
	log.Println("updating board w/ ID: " + strconv.FormatUint(turnToEnter.ID, 10) + ", new PGN: " + turnToEnter.PGN)

	filter := bson.D{{Key: "id", Value: turnToEnter.ID}}
	update := bson.D{{Key: "$set", Value: turnToEnter}}
	client = connectToMongoDB()
	result, err := collection.UpdateOne(context.TODO(), filter, update)
	log.Println("update attempt complete")
	client.Disconnect(context.TODO())
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		panic(err)
	} else {
		c.JSON(http.StatusOK, result)
	}

}

// returns PGN tied to id
func retrieveGameState(c *gin.Context) {
	client = connectToMongoDB()
	gameId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
	}
	log.Println("the id")
	log.Println(gameId)
	fmt.Printf("gameId: %T\n", gameId)
	log.Println("Attempting board retrieval")
	filter := bson.D{{Key: "id", Value: gameId}}
	singleResult := collection.FindOne(context.TODO(), filter)
	client.Disconnect(context.TODO())
	if singleResult.Err() != nil {
		//todo: resolve bug
		//bug is that result alwasy comes back as not found
		log.Println(singleResult.Err())
		c.JSON(http.StatusNotFound, "Game Not Found")
		//todo: different error codes based on singleResult.Err() value
	} else {
		var BSONData *ChessGame
		BSONData = new(ChessGame)
		err := singleResult.Decode(&BSONData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, err)
			panic(err)

		} else {
			log.Println(BSONData)
			log.Println("retreival successful")
			c.JSON(http.StatusOK, BSONData)
		}
	}
}

// deletes record tied to id
func endGame(c *gin.Context) {
	handleGameEnd(c, false)
}

func quitGame(c *gin.Context) {
	handleGameEnd(c, true)
}

func handleGameEnd(c *gin.Context, isQuit bool) {
	client = connectToMongoDB()
	gameId, err1 := strconv.Atoi(c.Param("id"))
	endingPlayer := c.Param("endingPlayer")
	var endGame *EndGameResponse = new(EndGameResponse)

	if err1 != nil {
		endGame.Success = false
		endGame.FailureMessage = err1.Error()
		c.JSON(http.StatusInternalServerError, endGame)
		panic(err1)
	}

	log.Println("attempting deletion of game record")
	filter := bson.D{{Key: "id", Value: gameId}}
	result, err := collection.DeleteOne(context.TODO(), filter)
	log.Println(result)
	log.Println("Delete operation finished")
	client.Disconnect(context.TODO())

	if err != nil {
		endGame.Success = false
		endGame.FailureMessage = err.Error()
		c.JSON(http.StatusInternalServerError, endGame)
		panic(err)
	} else {

		endGame.Success = true
		endGame.EndingPlayer = endingPlayer
		endGame.IsQuit = !isQuit
		c.JSON(http.StatusOK, endGame)
	}
}

// found from https://techwasti.com/cors-handling-in-go-gin-framework
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
