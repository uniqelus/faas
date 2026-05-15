@api-gateway
Feature: API Gateway health endpoints
  API gateway exposes admin health and metrics endpoints.

  Scenario: Admin endpoints are healthy
    Given api gateway is running
    When I request admin endpoint "/healthz"
    Then response status should be 200
    When I request admin endpoint "/metrics"
    Then response status should be 200
    And response contains "# HELP"
