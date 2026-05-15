@control-plane
Feature: Control-plane health and connectivity
  Control-plane should expose admin endpoints and accept gRPC connections.

  Scenario: Control-plane admin and gRPC are available
    Given control-plane is running
    Then control-plane gRPC port is reachable
    When I request control-plane admin endpoint "/healthz"
    Then response status should be 200
    When I request control-plane admin endpoint "/metrics"
    Then response status should be 200
    And response contains "# HELP"
