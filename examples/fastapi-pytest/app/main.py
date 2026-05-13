from fastapi import FastAPI, Depends, HTTPException
from sqlalchemy.orm import Session
from .database import get_db
from . import models

app = FastAPI()


@app.get("/users")
def list_users(db: Session = Depends(get_db)):
    return [
        {"id": u.id, "name": u.name, "email": u.email}
        for u in db.query(models.User).order_by(models.User.id)
    ]


@app.get("/users/{user_id}")
def get_user(user_id: int, db: Session = Depends(get_db)):
    user = db.get(models.User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="not found")
    return {"id": user.id, "name": user.name, "email": user.email}


@app.delete("/users/{user_id}", status_code=204)
def delete_user(user_id: int, db: Session = Depends(get_db)):
    user = db.get(models.User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="not found")
    db.delete(user)
    db.commit()


@app.get("/posts")
def list_posts(db: Session = Depends(get_db)):
    return [
        {"id": p.id, "user_id": p.user_id, "title": p.title}
        for p in db.query(models.Post).order_by(models.Post.id)
    ]
