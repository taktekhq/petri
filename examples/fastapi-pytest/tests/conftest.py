import pytest
from sqlalchemy import create_engine
from sqlalchemy.orm import Session
from fastapi.testclient import TestClient
from app.main import app
from app.database import get_db, db_url


@pytest.fixture
def client():
    # pool_size=1: all requests in this test share one TCP connection = one petri fork.
    engine = create_engine(db_url(), pool_size=1, max_overflow=0)

    def override():
        with Session(engine) as s:
            yield s

    app.dependency_overrides[get_db] = override
    with TestClient(app) as c:
        yield c
    engine.dispose()  # drops the connection; petri drops the fork
    app.dependency_overrides.clear()
