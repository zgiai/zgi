import json
from typing import Any
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from sqlalchemy.orm import Session
from sqlmodel import select
from starlette.responses import StreamingResponse

from app.core.exceptions import UnauthorizedError
from app.core.exceptions.flow import FlowOnlineEditError
from app.services.flow import FlowService
from app.services.auth import AuthService
from app.core.security import get_current_user
from app.utils.flow import build_flow_no_yield, get_L2_param_from_flow, remove_api_keys
from app.schemas.flow import (FlowCompareReq, FlowListRead, FlowVersionCreate, StreamData,
                          UnifiedResponseModel, resp_200)
from app.db.session import get_db
from app.models.flow import (Flow, FlowCreate, FlowDao, FlowRead, FlowReadWithStyle, FlowType,
                         FlowUpdate)
from app.models.flow_version import FlowVersionDao
from app.models.role_access import AccessType
from app.core.config import settings
from app.core.logger import logger
from app.schemas.user import UserInDB

# build router
router = APIRouter(prefix='/flows', tags=['Flows'])

@router.post('/', status_code=201)
def create_flow(
    *,
    request: Request,
    flow: FlowCreate,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Create a new flow."""
    # 判断用户是否重复技能名
    if db.query(Flow).filter(Flow.name == flow.name, Flow.user_id == current_user.id).first():
        raise HTTPException(status_code=500, detail='技能名重复')
    
    flow.user_id = current_user.id
    db_flow = Flow.model_validate(flow)
    # 创建新的技能
    db_flow = FlowDao.create_flow(db_flow, FlowType.FLOW.value)

    current_version = FlowVersionDao.get_version_by_flow(db_flow.id.hex)
    ret = FlowRead.model_validate(db_flow)
    ret.version_id = current_version.id
    FlowService.create_flow_hook(request, current_user, db_flow, ret.version_id)
    return resp_200(data=ret)


@router.get('/versions', status_code=200)
def get_versions(
    *,
    flow_id: UUID,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取技能对应的版本列表"""
    flow_id = flow_id.hex
    return FlowService.get_version_list_by_flow(current_user, flow_id)


@router.post('/versions', status_code=200)
def create_versions(
    *,
    flow_id: UUID,
    flow_version: FlowVersionCreate,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """创建新的技能版本"""
    flow_id = flow_id.hex
    return FlowService.create_new_version(current_user, flow_id, flow_version)


@router.put('/versions/{version_id}', status_code=200)
def update_versions(
    *,
    request: Request,
    version_id: int,
    flow_version: FlowVersionCreate,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """更新版本"""
    return FlowService.update_version_info(request, current_user, version_id, flow_version)


@router.delete('/versions/{version_id}', status_code=200)
def delete_versions(
    *,
    version_id: int,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """删除版本"""
    return FlowService.delete_version(current_user, version_id)


@router.get('/versions/{version_id}', status_code=200)
def get_version_info(
    *,
    version_id: int,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取版本信息"""
    return FlowService.get_version_info(current_user, version_id)


@router.post('/change_version', status_code=200)
def change_version(
    *,
    request: Request,
    flow_id: UUID = Query(default=None, description='技能唯一ID'),
    version_id: int = Query(default=None, description='需要设置的当前版本ID'),
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """修改当前版本"""
    flow_id = flow_id.hex
    return FlowService.change_current_version(request, current_user, flow_id, version_id)


@router.get('/', status_code=200)
def read_flows(
    *,
    name: str = Query(default=None, description='根据name查找数据库，包含描述的模糊搜索'),
    tag_id: int = Query(default=None, description='标签ID'),
    page_size: int = Query(default=10, description='每页数量'),
    page_num: int = Query(default=1, description='页数'),
    status: int = None,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Read all flows."""
    try:
        return FlowService.get_all_flows(current_user, name, status, tag_id, page_num, page_size)
    except Exception as e:
        logger.exception(e)
        raise HTTPException(status_code=500, detail=str(e)) from e


@router.get('/{flow_id}', response_model=UnifiedResponseModel[FlowReadWithStyle], status_code=200)
def read_flow(
    *,
    flow_id: UUID,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Read a flow."""
    return FlowService.get_one_flow(current_user, flow_id.hex)


@router.patch('/{flow_id}', response_model=UnifiedResponseModel[FlowRead], status_code=200)
async def update_flow(
    *,
    request: Request,
    flow_id: UUID,
    flow: FlowUpdate,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Update a flow."""
    flow_id = flow_id.hex
    with db as session:
        db_flow = session.get(Flow, flow_id)
    if not db_flow:
        raise HTTPException(status_code=404, detail='Flow not found')

    if not current_user.access_check(db_flow.user_id, flow_id, AccessType.FLOW_WRITE):
        return UnauthorizedError.return_resp()

    flow_data = flow.model_dump(exclude_unset=True)

    if 'status' in flow_data and flow_data['status'] == 2 and db_flow.status == 1:
        # 上线校验
        try:
            art = {}
            await build_flow_no_yield(graph_data=db_flow.data,
                                      artifacts=art,
                                      process_file=False,
                                      flow_id=flow_id)
        except Exception as exc:
            logger.exception(exc)
            raise HTTPException(status_code=500, detail=f'Flow build error, {str(exc)}')

    if db_flow.status == 2 and ('status' not in flow_data or flow_data['status'] != 1):
        raise FlowOnlineEditError.http_exception()

    if settings.remove_api_keys:
        flow_data = remove_api_keys(flow_data)
    for key, value in flow_data.items():
        setattr(db_flow, key, value)
    with db as session:
        session.add(db_flow)
        session.commit()
        session.refresh(db_flow)
    try:
        if not get_L2_param_from_flow(db_flow.data, db_flow.id):
            logger.error(f'flow_id={db_flow.id} extract file_node fail')
    except Exception:
        pass
    FlowService.update_flow_hook(request, current_user, db_flow)
    return resp_200(db_flow)


@router.delete('/{flow_id}', status_code=200)
def delete_flow(
    *,
    request: Request,
    flow_id: UUID,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Delete a flow."""
    flow_id = flow_id.hex
    with db as session:
        db_flow = session.get(Flow, flow_id)
    if not db_flow:
        raise HTTPException(status_code=404, detail='Flow not found')
    if not current_user.access_check(db_flow.user_id, flow_id, AccessType.FLOW_WRITE):
        return UnauthorizedError.return_resp()
    FlowService.delete_flow_hook(request, current_user, db_flow)
    with db as session:
        session.delete(db_flow)
        session.commit()
    return resp_200()


@router.get('/download', status_code=200)
def download_file(
    *,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Download all flows as a file."""
    with db as session:
        flows = session.exec(select(Flow)).all()
    return StreamingResponse(iter([str(flows)]),
                           media_type='application/octet-stream',
                           headers={'Content-Disposition': 'attachment; filename=flows.txt'})


@router.post('/compare', status_code=200)
def compare_flow_node(
    *,
    item: FlowCompareReq,
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """技能多版本对比"""
    return FlowService.compare_flow_node(current_user, item)


@router.get('/compare/stream', status_code=200)
def compare_flow_node_stream(
    *,
    data: Any = Query(description='对比所需数据的json序列化后的字符串'),
    current_user: UserInDB = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """技能多版本对比"""
    data = json.loads(data)
    item = FlowCompareReq(**data)

    async def generate():
        async for chunk in FlowService.compare_flow_node_stream(current_user, item):
            if chunk:
                yield StreamData(data=chunk).model_dump_json() + '\n'

    return StreamingResponse(generate(), media_type='text/event-stream')
