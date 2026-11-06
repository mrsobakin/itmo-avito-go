import asyncio
import uuid
from collections import Counter

import httpx

N = 20


def trip():
    return {
        "user_id": str(uuid.uuid4()),
        "driver_id": str(uuid.uuid4()),
        "start_point": {"latitude": 59.9398, "longitude": 30.3146},
        "end_point": {"latitude": 59.9290, "longitude": 30.3626},
        "price": 1450,
    }


async def race(title, make_request):
    responses = await asyncio.gather(*(make_request() for _ in range(N)))
    print(title, dict(Counter(response.status_code for response in responses)))


async def main():
    async with httpx.AsyncClient(base_url="http://localhost:8080") as http:
        body = trip()
        await race("create same driver", lambda: http.post("/api/v1/trips", json=body))

        trip_id = (await http.post("/api/v1/trips", json=trip())).json()["id"]
        await race("finish same trip", lambda: http.post(f"/api/v1/trips/{trip_id}/finish"))

        body = trip()
        key = {"Idempotency-Key": str(uuid.uuid4())}
        await race("same idempotency key", lambda: http.post("/api/v1/trips", json=body, headers=key))


asyncio.run(main())
