# 构建阶段
FROM node:16 as build-stage
WORKDIR /app
COPY package*.json ./
# 安装依赖时添加错误处理
RUN npm install --no-fund --no-audit --loglevel=error || (cat npm-debug.log && exit 1)
COPY . .
# 构建时添加错误处理
RUN npm run build || (cat npm-debug.log && exit 1)

# 生产阶段
FROM nginx:stable-alpine as production-stage
COPY --from=build-stage /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]