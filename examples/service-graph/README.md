# Instrumented checkout example

Three small Go HTTP services make real calls: gateway → checkout → payments. A bounded generator sends a request every 500 ms. Each service exposes its own request counters; an explicit outbound HTTP wrapper exposes `traces_service_graph_request_total`. This example has no trace processor and does not claim instrumentation coverage beyond its three known services.

Follow the hosted [service dependency guide](../../docs/public/service-dependencies.md#run-an-instrumented-example) for the optional Compose overlay, SLOs, critical user journey and reviewed dependency map. The base playground does not start these services. With Mimir enabled, Prometheus scrapes and remote-writes their counters to organization `playground`; apply the definitions to the intended `prometheus` or `mimir` Nines tenant.

`make test-topology` creates a separate private Docker network, builds only this example, starts its own Prometheus and verifies real calls, actual sample timestamps, journey steps, tenant boundaries and stale evidence after pausing its own checkout exporter. Its cleanup touches only the disposable fixture.
