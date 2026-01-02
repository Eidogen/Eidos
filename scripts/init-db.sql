-- 创建各服务的数据库
CREATE DATABASE eidos_trading;
CREATE DATABASE eidos_chain;
CREATE DATABASE eidos_risk;
CREATE DATABASE eidos_admin;

-- 授权
GRANT ALL PRIVILEGES ON DATABASE eidos_trading TO eidos;
GRANT ALL PRIVILEGES ON DATABASE eidos_chain TO eidos;
GRANT ALL PRIVILEGES ON DATABASE eidos_risk TO eidos;
GRANT ALL PRIVILEGES ON DATABASE eidos_admin TO eidos;
