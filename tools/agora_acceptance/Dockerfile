FROM debian:13-slim
RUN apt-get update && apt-get install -y ca-certificates curl && mkdir -p /etc/apt/keyrings && curl -fsSL https://forgejo.ellis.link/api/packages/continuwuation/debian/repository.key -o /etc/apt/keyrings/continuwuity.asc && echo 'deb [signed-by=/etc/apt/keyrings/continuwuity.asc] https://forgejo.ellis.link/api/packages/continuwuation/debian trixie stable' > /etc/apt/sources.list.d/continuwuity.list && apt-get update && apt-get install -y continuwuity=26.9.1
ENTRYPOINT ["/usr/bin/conduwuit"]
