import asyncio
import json
import uuid

import httpx


def trip(driver_id, price=1450, latitude=59.9398):
    return {
        "user_id": str(uuid.uuid4()),
        "driver_id": driver_id,
        "start_point": {"latitude": latitude, "longitude": 30.3146},
        "end_point": {"latitude": 59.9290, "longitude": 30.3626},
        "price": price,
    }


def do(title, response):
    print(title, response.status_code, response.text.strip())
    return response.json()


async def main():
    async with httpx.AsyncClient(base_url="http://localhost:8080") as http:
        driver_id = str(uuid.uuid4())
        body = trip(driver_id)

        do("health", await http.get("/health"))
        do("ready", await http.get("/ready"))

        trip_id = do("create", await http.post("/api/v1/trips", json=body))["id"]
        do("create same driver", await http.post("/api/v1/trips", json=body))
        do("get", await http.get(f"/api/v1/trips/{trip_id}"))
        do("finish", await http.post(f"/api/v1/trips/{trip_id}/finish"))
        do("finish again", await http.post(f"/api/v1/trips/{trip_id}/finish"))
        do("get missing", await http.get(f"/api/v1/trips/{uuid.uuid4()}"))
        do("get bad id", await http.get("/api/v1/trips/abc"))

        do("bad latitude", await http.post("/api/v1/trips", json=trip(str(uuid.uuid4()), latitude=91)))
        do("unknown field", await http.post("/api/v1/trips", json={**trip(str(uuid.uuid4())), "foo": 1}))
        do("trailing data", await http.post(
            "/api/v1/trips",
            content=json.dumps(trip(str(uuid.uuid4()))) + " {}",
            headers={"Content-Type": "application/json"},
        ))

        key = {"Idempotency-Key": str(uuid.uuid4())}
        body = trip(str(uuid.uuid4()))

        do("idempotent create", await http.post("/api/v1/trips", json=body, headers=key))
        do("idempotent repeat", await http.post("/api/v1/trips", json=body, headers=key))
        do("idempotent conflict", await http.post("/api/v1/trips", json={**body, "price": 1}, headers=key))


asyncio.run(main())
