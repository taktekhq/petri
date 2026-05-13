def test_cascade_delete_is_isolated(client):
    assert client.delete("/users/1").status_code == 204
    assert all(p["user_id"] != 1 for p in client.get("/posts").json())


def test_seeded_posts(client):
    assert len(client.get("/posts").json()) == 3
