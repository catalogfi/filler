.PHONY: build run stop logs status restart

COMPOSE_FILE=docker-compose.yml
ENV_FILE=.env
DOCKER_COMPOSE=docker-compose

all: build

define print_message
	@echo "\033[1;34m$(1)\033[0m"
endef

# Build and start cobi containers
build:
	$(call print_message, "Building and starting the containers")
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) -f $(COMPOSE_FILE) up --build -d

# Start cobi with existing image
run:
	$(call print_message, "Starting the containers")
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) -f $(COMPOSE_FILE) up -d

# Stop cobi containers
stop:
	$(call print_message, "Stopping the containers")
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) -f $(COMPOSE_FILE) down

# View logs of the containers
logs:
	$(call print_message, "Displaying logs of the containers")
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) -f $(COMPOSE_FILE) logs -f

# Check the status of the containers
status:
	$(call print_message, "Checking the status of the containers")
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) -f $(COMPOSE_FILE) ps

# Restart the containers
restart:
	$(call print_message, "Restarting the containers")
	@$(MAKE) stop
	@$(MAKE) run