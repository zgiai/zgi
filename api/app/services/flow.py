from typing import List, Optional
from sqlalchemy.orm import Session
from fastapi import HTTPException, status

from app.models.flow import Flow, FlowCreate, FlowRead, FlowReadWithStyle
from app.models.flow_version import FlowVersion
from app.schemas.user import UserInDB


class FlowService:
    @staticmethod
    def create_flow_hook(request, user: UserInDB, flow: Flow, version_id: int):
        """Flow creation hook"""
        pass

    @staticmethod
    def update_flow_hook(request, user: UserInDB, flow: Flow):
        """Flow update hook"""
        pass

    @staticmethod
    def delete_flow_hook(request, user: UserInDB, flow: Flow):
        """Flow deletion hook"""
        pass

    @staticmethod
    def get_version_list_by_flow(user: UserInDB, flow_id: str):
        """Get version list by flow id"""
        pass

    @staticmethod
    def create_new_version(user: UserInDB, flow_id: str, version_data):
        """Create new version"""
        pass

    @staticmethod
    def update_version_info(request, user: UserInDB, version_id: int, version_data):
        """Update version info"""
        pass

    @staticmethod
    def delete_version(user: UserInDB, version_id: int):
        """Delete version"""
        pass

    @staticmethod
    def get_version_info(user: UserInDB, version_id: int):
        """Get version info"""
        pass

    @staticmethod
    def change_current_version(request, user: UserInDB, flow_id: str, version_id: int):
        """Change current version"""
        pass

    @staticmethod
    def get_all_flows(user: UserInDB, name: Optional[str] = None, status: Optional[int] = None,
                    tag_id: Optional[int] = None, page_num: int = 1, page_size: int = 10):
        """Get all flows"""
        pass

    @staticmethod
    def get_one_flow(user: UserInDB, flow_id: str):
        """Get one flow"""
        pass

    @staticmethod
    def compare_flow_node(user: UserInDB, item):
        """Compare flow node"""
        pass

    @staticmethod
    async def compare_flow_node_stream(user: UserInDB, item):
        """Compare flow node stream"""
        async def generate():
            yield "Comparison result"
        return generate()
