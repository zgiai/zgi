from app.utils.mysql_manager import MySQLManager


async def get_user_info(user_id: int) -> dict:
    with MySQLManager() as db:
        query = "SELECT id, name, email, created_at FROM users WHERE id = %s"
        results = db.execute_query(query, (user_id,))
        return results[0]