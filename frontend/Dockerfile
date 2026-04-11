FROM node:24-alpine AS build

WORKDIR /app

ARG API_BASE_URL=http://localhost:8080/api/v1
ENV VITE_API_BASE_URL=${API_BASE_URL}

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginx:1.29-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 5173

CMD ["nginx", "-g", "daemon off;"]
