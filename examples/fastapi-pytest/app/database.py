import os
from sqlalchemy import create_engine
from sqlalchemy.orm import Session


def db_url() -> str:
    port = os.environ.get("PORT", "5432")
    return f"postgresql://postgres:postgres@postgres:{port}/postgres"


def get_db():
    engine = create_engine(db_url())
    with Session(engine) as session:
        yield session
    engine.dispose()
