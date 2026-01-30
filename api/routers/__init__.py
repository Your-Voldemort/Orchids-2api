from fastapi import APIRouter
from api.routers.tasks import router as tasks_router
from api.routers.evaluate import router as evaluate_router
from api.routers.accounts import router as accounts_router
from api.routers.knowledge import router as knowledge_router
from api.routers.textbook import router as textbook_router
from api.routers.video import router as video_router
from api.routers.claude import router as claude_router
from api.routers.mcp import router as mcp_router
from api.routers.rpg import router as rpg_router
from api.routers.school_sim import router as school_sim_router
from api.routers.messages import router as messages_router
from api.routers.story import router as story_router
from api.routers.z_image import router as z_image_router
from api.routers.special_script import router as special_script_router
from api.routers.v1 import router as v1_router

api_router = APIRouter()

# Register V1 Proxy separately if needed, or include in prefix
api_router.include_router(v1_router, prefix="/v1", tags=["V1 Proxy"])

# Feature routers
api_router.include_router(tasks_router, prefix="/api/tasks", tags=["Tasks"])
api_router.include_router(evaluate_router, prefix="/api/evaluate", tags=["Evaluate"])
api_router.include_router(accounts_router, prefix="/api/accounts", tags=["Accounts"])
api_router.include_router(knowledge_router, prefix="/api/knowledge", tags=["Knowledge"])
api_router.include_router(textbook_router, prefix="/api/textbook", tags=["Textbook"])
api_router.include_router(video_router, prefix="/api/video", tags=["Video"])
api_router.include_router(claude_router, prefix="/api/claude", tags=["Claude"])
api_router.include_router(mcp_router, prefix="/api/mcp", tags=["MCP"])
api_router.include_router(rpg_router, prefix="/api/rpg", tags=["RPG"])
api_router.include_router(school_sim_router, prefix="/api/school_sim", tags=["SchoolSim"])
api_router.include_router(messages_router, prefix="/api/messages", tags=["Messages"])
api_router.include_router(story_router, prefix="/api/story", tags=["Story"])
api_router.include_router(z_image_router, prefix="/api/z_image", tags=["ZImage"])
api_router.include_router(special_script_router, prefix="/api/special_script", tags=["SpecialScript"])
