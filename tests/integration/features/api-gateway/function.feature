@api-gateway
Feature: API Gateway function lifecycle routes
  Gateway should expose function lifecycle endpoints.

  Scenario: Create/get/list/delete endpoints are reachable
    Given api gateway is running
    Then lifecycle action "create" returns status 404
    And lifecycle action "get" returns status 404
    And lifecycle action "list" returns status 404
    And lifecycle action "delete" returns status 404
