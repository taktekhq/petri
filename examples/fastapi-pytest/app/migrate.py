from sqlalchemy import create_engine
from sqlalchemy.orm import Session
from .models import Base, User, Post

# Always seeds on :5432 (petri passthrough); forks on :5433 inherit this state.
_URL = "postgresql://postgres:postgres@postgres:5432/postgres"


def run():
    engine = create_engine(_URL)
    Base.metadata.drop_all(engine)
    Base.metadata.create_all(engine)
    with Session(engine) as s:
        alice = User(email="alice@example.com", name="Alice")
        bob = User(email="bob@example.com", name="Bob")
        s.add_all([alice, bob])
        s.flush()
        s.add_all([
            Post(user_id=alice.id, title="Hello from Alice"),
            Post(user_id=alice.id, title="Alice again"),
            Post(user_id=bob.id, title="Hello from Bob"),
        ])
        s.commit()
    engine.dispose()


if __name__ == "__main__":
    run()
