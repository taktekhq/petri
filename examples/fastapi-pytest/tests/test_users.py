def test_seeded_users(client):
    assert len(client.get("/users").json()) == 2


def test_delete_is_isolated(client):
    assert client.delete("/users/2").status_code == 204
    assert client.get("/users/2").status_code == 404
