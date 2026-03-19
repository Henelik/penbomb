package main

import (
	"github.com/Henelik/penbomb/handlers"
	"github.com/gofiber/fiber/v2"
)

func main() {
	app := fiber.New()

	handlers.RegisterSuggestedRoutes(app)

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
